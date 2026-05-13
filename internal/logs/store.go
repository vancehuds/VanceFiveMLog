package logs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidEvent = errors.New("invalid log event")
var ErrInvalidReview = errors.New("invalid log review")

const (
	maxReviewNoteLength      = 4096
	retentionDeleteBatchSize = 5000
)

var auditActionPattern = regexp.MustCompile(`^[a-z0-9_.:-]{1,80}$`)

type Store struct {
	db *pgxpool.Pool
}

type Stats struct {
	TodayTotal int64
	Warnings   int64
	Errors     int64
	Servers    int64
}

type AccountLink struct {
	License    string
	Names      []string
	Discords   []string
	Steams     []string
	CitizenIDs []string
	Events     int64
	LastSeen   time.Time
}

type PlayerSummary struct {
	Events    int64
	Warnings  int64
	Errors    int64
	Resources int64
	LastSeen  *time.Time
}

type insertEventRow struct {
	ServerID     int64           `json:"server_id"`
	EventType    string          `json:"event_type"`
	Severity     string          `json:"severity"`
	PlayerSource *int            `json:"player_source"`
	PlayerName   *string         `json:"player_name"`
	License      *string         `json:"license"`
	Discord      *string         `json:"discord"`
	Steam        *string         `json:"steam"`
	CitizenID    *string         `json:"citizenid"`
	Resource     *string         `json:"resource"`
	Message      string          `json:"message"`
	CoordsX      *float64        `json:"coords_x"`
	CoordsY      *float64        `json:"coords_y"`
	CoordsZ      *float64        `json:"coords_z"`
	Metadata     json.RawMessage `json:"metadata"`
	OccurredAt   time.Time       `json:"occurred_at"`
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) InsertEvents(ctx context.Context, serverID int64, incoming []IngestEvent) ([]Event, error) {
	if len(incoming) == 0 {
		return nil, ErrInvalidEvent
	}
	if len(incoming) > 500 {
		return nil, fmt.Errorf("%w: batch limit is 500", ErrInvalidEvent)
	}

	rows, err := prepareInsertEventRows(serverID, incoming, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}
	resultRows, err := s.db.Query(ctx, `
		WITH incoming AS (
			SELECT *
			FROM jsonb_to_recordset($1::jsonb) AS x(
				server_id bigint,
				event_type text,
				severity text,
				player_source integer,
				player_name text,
				license text,
				discord text,
				steam text,
				citizenid text,
				resource text,
				message text,
				coords_x double precision,
				coords_y double precision,
				coords_z double precision,
				metadata jsonb,
				occurred_at timestamptz
			)
		)
		INSERT INTO log_events (
			server_id, event_type, severity, player_source, player_name, license,
			discord, steam, citizenid, resource, message, coords_x, coords_y, coords_z,
			metadata, occurred_at
		)
		SELECT
			server_id, event_type, severity, player_source, player_name, license,
			discord, steam, citizenid, resource, message, coords_x, coords_y, coords_z,
			metadata, occurred_at
		FROM incoming
		RETURNING id, server_id, '' AS server_name, event_type, severity, player_source,
			coalesce(player_name, ''), coalesce(license, ''), coalesce(discord, ''),
			coalesce(steam, ''), coalesce(citizenid, ''), coalesce(resource, ''), message,
			coords_x, coords_y, coords_z, metadata, occurred_at, created_at,
			'normal' AS review_status, '' AS review_note, NULL::timestamptz AS archived_at,
			NULL::bigint AS archived_by, NULL::bigint AS updated_by, NULL::timestamptz AS updated_at,
			'' AS archived_by_name, '' AS updated_by_name
	`, string(payload))
	if err != nil {
		return nil, err
	}
	defer resultRows.Close()
	return scanEvents(resultRows)
}

func prepareInsertEventRows(serverID int64, incoming []IngestEvent, now time.Time) ([]insertEventRow, error) {
	rows := make([]insertEventRow, 0, len(incoming))
	for i := range incoming {
		if err := incoming[i].Normalize(now); err != nil {
			if errors.Is(err, ErrInvalidEvent) {
				// Skip malformed plugin events instead of rejecting a whole mixed batch.
				continue
			}
			return nil, err
		}
		meta, err := json.Marshal(incoming[i].Metadata)
		if err != nil {
			return nil, err
		}
		var x, y, z *float64
		if incoming[i].Coords != nil {
			x, y, z = &incoming[i].Coords.X, &incoming[i].Coords.Y, &incoming[i].Coords.Z
		}
		rows = append(rows, insertEventRow{
			ServerID:     serverID,
			EventType:    incoming[i].EventType,
			Severity:     incoming[i].Severity,
			PlayerSource: incoming[i].Source,
			PlayerName:   nullableString(incoming[i].PlayerName),
			License:      nullableString(incoming[i].License),
			Discord:      nullableString(incoming[i].Discord),
			Steam:        nullableString(incoming[i].Steam),
			CitizenID:    nullableString(incoming[i].CitizenID),
			Resource:     nullableString(incoming[i].Resource),
			Message:      incoming[i].Message,
			CoordsX:      x,
			CoordsY:      y,
			CoordsZ:      z,
			Metadata:     meta,
			OccurredAt:   *incoming[i].OccurredAt,
		})
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: no valid events in batch", ErrInvalidEvent)
	}
	return rows, nil
}

func (s *Store) Query(ctx context.Context, q Query) ([]Event, error) {
	sql, args := querySQL(q)
	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (s *Store) Get(ctx context.Context, id int64) (Event, error) {
	if id <= 0 {
		return Event{}, pgx.ErrNoRows
	}
	rows, err := s.db.Query(ctx, `
		SELECT e.id, e.server_id, s.name, e.event_type, e.severity, e.player_source,
			coalesce(e.player_name, ''),
			coalesce(e.license, ''), coalesce(e.discord, ''), coalesce(e.steam, ''),
			coalesce(e.citizenid, ''), coalesce(e.resource, ''), e.message,
			e.coords_x, e.coords_y, e.coords_z, e.metadata, e.occurred_at, e.created_at,
			coalesce(r.status, 'normal'), coalesce(r.note, ''), r.archived_at, r.archived_by,
			r.updated_by, r.updated_at, coalesce(archived_admin.username, ''),
			coalesce(updated_admin.username, '')
		FROM log_events e
		JOIN servers s ON s.id = e.server_id
		LEFT JOIN log_event_reviews r ON r.event_id = e.id
		LEFT JOIN admins archived_admin ON archived_admin.id = r.archived_by
		LEFT JOIN admins updated_admin ON updated_admin.id = r.updated_by
		WHERE e.id = $1
	`, id)
	if err != nil {
		return Event{}, err
	}
	defer rows.Close()

	events, err := scanEvents(rows)
	if err != nil {
		return Event{}, err
	}
	if len(events) == 0 {
		return Event{}, pgx.ErrNoRows
	}
	return events[0], nil
}

func querySQL(q Query) (string, []any) {
	q = normalizeQuery(q)
	var args []any
	where := queryWhere(q, &args)
	args = append(args, q.Limit, q.Offset)
	limitPos := len(args) - 1
	offsetPos := len(args)
	return fmt.Sprintf(`
		WITH selected AS (
			SELECT e.id
			FROM log_events e
			WHERE %s
			ORDER BY e.occurred_at DESC, e.id DESC
			LIMIT $%d OFFSET $%d
		)
		SELECT e.id, e.server_id, s.name, e.event_type, e.severity, e.player_source,
			coalesce(e.player_name, ''),
			coalesce(e.license, ''), coalesce(e.discord, ''), coalesce(e.steam, ''),
			coalesce(e.citizenid, ''), coalesce(e.resource, ''), e.message,
			e.coords_x, e.coords_y, e.coords_z, e.metadata, e.occurred_at, e.created_at,
			coalesce(r.status, 'normal'), coalesce(r.note, ''), r.archived_at, r.archived_by,
			r.updated_by, r.updated_at, coalesce(archived_admin.username, ''),
			coalesce(updated_admin.username, '')
		FROM selected
		JOIN log_events e ON e.id = selected.id
		JOIN servers s ON s.id = e.server_id
		LEFT JOIN log_event_reviews r ON r.event_id = e.id
		LEFT JOIN admins archived_admin ON archived_admin.id = r.archived_by
		LEFT JOIN admins updated_admin ON updated_admin.id = r.updated_by
		ORDER BY e.occurred_at DESC, e.id DESC
	`, strings.Join(where, " AND "), limitPos, offsetPos), args
}

func (s *Store) Count(ctx context.Context, q Query) (int64, error) {
	sql, args := countSQL(q)
	var total int64
	err := s.db.QueryRow(ctx, sql, args...).Scan(&total)
	return total, err
}

func countSQL(q Query) (string, []any) {
	q = normalizeQuery(q)
	var args []any
	where := queryWhere(q, &args)
	return fmt.Sprintf(`
		SELECT count(*)
		FROM log_events e
		WHERE %s
	`, strings.Join(where, " AND ")), args
}

func (s *Store) DashboardStats(ctx context.Context, loc *time.Location) (Stats, error) {
	if loc == nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	last24 := time.Now().UTC().Add(-24 * time.Hour)
	cutoff := dayStart
	if last24.Before(cutoff) {
		cutoff = last24
	}
	var stats Stats
	err := s.db.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE occurred_at >= $1),
			count(*) FILTER (WHERE severity = 'warning' AND occurred_at >= $2),
			count(*) FILTER (WHERE severity = 'error' AND occurred_at >= $2),
			(SELECT count(*) FROM servers WHERE active = true)
		FROM log_events
		WHERE occurred_at >= $3
	`, dayStart, last24, cutoff).Scan(&stats.TodayTotal, &stats.Warnings, &stats.Errors, &stats.Servers)
	return stats, err
}

func (s *Store) HourlyBuckets(ctx context.Context, hours int, loc *time.Location) ([]HourBucket, error) {
	if hours < 1 {
		hours = 24
	}
	if hours > 168 {
		hours = 168
	}
	if loc == nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	end := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, loc)
	start := end.Add(time.Duration(-(hours - 1)) * time.Hour)
	rows, err := s.db.Query(ctx, `
		WITH buckets AS (
			SELECT generate_series(
				$1::timestamptz,
				$2::timestamptz,
				'1 hour'::interval
			) AS hour
		)
		SELECT
			b.hour,
			count(e.id) AS total,
			count(e.id) FILTER (WHERE e.severity = 'error') AS errors
		FROM buckets b
		LEFT JOIN log_events e
			ON e.occurred_at >= b.hour
			AND e.occurred_at < b.hour + '1 hour'::interval
			AND e.occurred_at >= $1
			AND e.occurred_at < $2::timestamptz + '1 hour'::interval
		GROUP BY b.hour
		ORDER BY b.hour
	`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var buckets []HourBucket
	for rows.Next() {
		var bucket HourBucket
		if err := rows.Scan(&bucket.Hour, &bucket.Total, &bucket.Errors); err != nil {
			return nil, err
		}
		buckets = append(buckets, bucket)
	}
	return buckets, rows.Err()
}

func (s *Store) TopEventTypes(ctx context.Context, limit int) ([]EventTypeCount, error) {
	if limit <= 0 {
		limit = 8
	}
	if limit > 50 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT event_type, max(severity) AS severity, count(*) AS total
		FROM log_events
		WHERE occurred_at >= now() - interval '24 hours'
		GROUP BY event_type
		ORDER BY total DESC, event_type ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var counts []EventTypeCount
	for rows.Next() {
		var count EventTypeCount
		if err := rows.Scan(&count.EventType, &count.Severity, &count.Total); err != nil {
			return nil, err
		}
		counts = append(counts, count)
	}
	return counts, rows.Err()
}

func (s *Store) PlayerSummary(ctx context.Context, player string) (PlayerSummary, error) {
	player = strings.TrimSpace(player)
	if player == "" {
		return PlayerSummary{}, nil
	}

	var summary PlayerSummary
	sql := fmt.Sprintf(`
		SELECT
			count(*) AS events,
			count(*) FILTER (WHERE severity = 'warning') AS warnings,
			count(*) FILTER (WHERE severity = 'error') AS errors,
			count(DISTINCT nullif(resource, '')) AS resources,
			max(occurred_at) AS last_seen
		FROM log_events e
		WHERE %s
	`, playerWhereClause(1, 2))
	err := s.db.QueryRow(ctx, sql, "%"+player+"%", player).Scan(&summary.Events, &summary.Warnings, &summary.Errors, &summary.Resources, &summary.LastSeen)
	return summary, err
}

func (s *Store) AccountLinks(ctx context.Context, keyword string) ([]AccountLink, error) {
	return s.AccountLinksPage(ctx, keyword, 100, 0)
}

func (s *Store) AccountLinksPage(ctx context.Context, keyword string, limit, offset int) ([]AccountLink, error) {
	keyword = strings.TrimSpace(keyword)
	limit, offset = normalizeLimitOffset(limit, offset)
	where := []string{"(license IS NOT NULL OR discord IS NOT NULL OR steam IS NOT NULL OR citizenid IS NOT NULL)"}
	var args []any
	if keyword != "" {
		args = append(args, "%"+keyword+"%")
		where = append(where, fmt.Sprintf(`(
			coalesce(player_name, '') ILIKE $%[1]d OR coalesce(license, '') ILIKE $%[1]d OR
			coalesce(discord, '') ILIKE $%[1]d OR coalesce(steam, '') ILIKE $%[1]d OR
			coalesce(citizenid, '') ILIKE $%[1]d
		)`, len(args)))
	}

	rows, err := s.db.Query(ctx, fmt.Sprintf(`
		SELECT
			coalesce(license, 'unknown') AS license,
			array_remove(array_agg(DISTINCT player_name), NULL) AS names,
			array_remove(array_agg(DISTINCT discord), NULL) AS discords,
			array_remove(array_agg(DISTINCT steam), NULL) AS steams,
			array_remove(array_agg(DISTINCT citizenid), NULL) AS citizenids,
			count(*) AS events,
			max(occurred_at) AS last_seen
		FROM log_events
		WHERE %s
		GROUP BY coalesce(license, 'unknown')
		ORDER BY last_seen DESC
		LIMIT $%d OFFSET $%d
	`, strings.Join(where, " AND "), len(args)+1, len(args)+2), append(args, limit, offset)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []AccountLink
	for rows.Next() {
		var link AccountLink
		if err := rows.Scan(&link.License, &link.Names, &link.Discords, &link.Steams, &link.CitizenIDs, &link.Events, &link.LastSeen); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func (s *Store) AccountLinksCount(ctx context.Context, keyword string) (int64, error) {
	keyword = strings.TrimSpace(keyword)
	where := []string{"(license IS NOT NULL OR discord IS NOT NULL OR steam IS NOT NULL OR citizenid IS NOT NULL)"}
	var args []any
	if keyword != "" {
		args = append(args, "%"+keyword+"%")
		where = append(where, fmt.Sprintf(`(
			coalesce(player_name, '') ILIKE $%[1]d OR coalesce(license, '') ILIKE $%[1]d OR
			coalesce(discord, '') ILIKE $%[1]d OR coalesce(steam, '') ILIKE $%[1]d OR
			coalesce(citizenid, '') ILIKE $%[1]d
		)`, len(args)))
	}

	var total int64
	err := s.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT count(*)
		FROM (
			SELECT 1
			FROM log_events
			WHERE %s
			GROUP BY coalesce(license, 'unknown')
		) accounts
	`, strings.Join(where, " AND ")), args...).Scan(&total)
	return total, err
}

func (s *Store) DeleteOlderThan(ctx context.Context, days int) (int64, error) {
	if days < 1 {
		days = 1
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	var total int64
	for {
		tag, err := s.db.Exec(ctx, `
			WITH expired AS (
				SELECT id
				FROM log_events
				WHERE occurred_at < $1
				ORDER BY occurred_at ASC, id ASC
				LIMIT $2
			)
			DELETE FROM log_events e
			USING expired
			WHERE e.id = expired.id
		`, cutoff, retentionDeleteBatchSize)
		if err != nil {
			return total, err
		}
		deleted := tag.RowsAffected()
		total += deleted
		if deleted < retentionDeleteBatchSize {
			return total, nil
		}
	}
}

func (s *Store) ReviewEvent(ctx context.Context, eventID, adminID int64, status, note string) error {
	if eventID <= 0 || adminID <= 0 {
		return ErrInvalidReview
	}
	status = NormalizeReviewStatus(status)
	if status == "" {
		return ErrInvalidReview
	}
	note = strings.TrimSpace(note)
	if len(note) > maxReviewNoteLength {
		note = truncateString(note, maxReviewNoteLength)
	}
	tag, err := s.db.Exec(ctx, `
		INSERT INTO log_event_reviews (event_id, status, note, updated_by, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (event_id) DO UPDATE
		SET status = EXCLUDED.status,
			note = EXCLUDED.note,
			updated_by = EXCLUDED.updated_by,
			updated_at = now()
	`, eventID, status, note, adminID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) ArchiveEvents(ctx context.Context, eventIDs []int64, adminID int64) (int64, error) {
	eventIDs = cleanEventIDs(eventIDs)
	if len(eventIDs) == 0 || adminID <= 0 {
		return 0, ErrInvalidReview
	}
	tag, err := s.db.Exec(ctx, `
		INSERT INTO log_event_reviews (event_id, status, archived_at, archived_by, updated_by, updated_at)
		SELECT e.id, 'normal', now(), $2, $2, now()
		FROM log_events e
		WHERE e.id = ANY($1)
		ON CONFLICT (event_id) DO UPDATE
		SET archived_at = coalesce(log_event_reviews.archived_at, now()),
			archived_by = coalesce(log_event_reviews.archived_by, EXCLUDED.archived_by),
			updated_by = EXCLUDED.updated_by,
			updated_at = now()
	`, eventIDs, adminID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) ArchiveQuery(ctx context.Context, q Query, adminID int64, maxRows int) (int64, error) {
	if adminID <= 0 {
		return 0, ErrInvalidReview
	}
	q = normalizeQuery(q)
	if maxRows <= 0 || maxRows > 5000 {
		maxRows = 5000
	}
	q.Limit = maxRows
	q.Offset = 0
	var args []any
	where := queryWhere(q, &args)
	args = append(args, adminID, maxRows)
	adminPos := len(args) - 1
	limitPos := len(args)
	tag, err := s.db.Exec(ctx, fmt.Sprintf(`
		WITH selected AS (
			SELECT e.id
			FROM log_events e
			WHERE %s
			ORDER BY e.occurred_at DESC, e.id DESC
			LIMIT $%d
		)
		INSERT INTO log_event_reviews (event_id, status, archived_at, archived_by, updated_by, updated_at)
		SELECT selected.id, 'normal', now(), $%d, $%d, now()
		FROM selected
		ON CONFLICT (event_id) DO UPDATE
		SET archived_at = coalesce(log_event_reviews.archived_at, now()),
			archived_by = coalesce(log_event_reviews.archived_by, EXCLUDED.archived_by),
			updated_by = EXCLUDED.updated_by,
			updated_at = now()
	`, strings.Join(where, " AND "), limitPos, adminPos, adminPos), args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) AuditAdminAction(ctx context.Context, entry AdminAuditEntry) error {
	return auditAdminAction(ctx, s.db, entry)
}

type auditExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func auditAdminAction(ctx context.Context, execer auditExecer, entry AdminAuditEntry) error {
	entry.Action = strings.ToLower(strings.TrimSpace(entry.Action))
	if entry.AdminID <= 0 || entry.AdminUsername == "" || !auditActionPattern.MatchString(entry.Action) {
		return ErrInvalidReview
	}
	entry.EventIDs = cleanEventIDs(entry.EventIDs)
	if entry.Query == nil {
		entry.Query = map[string]string{}
	}
	if entry.Details == nil {
		entry.Details = map[string]any{}
	}
	query, err := json.Marshal(entry.Query)
	if err != nil {
		return err
	}
	details, err := json.Marshal(entry.Details)
	if err != nil {
		return err
	}
	_, err = execer.Exec(ctx, `
		INSERT INTO log_admin_audit (admin_id, admin_username, action, event_id, event_ids, query, details)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb)
	`, entry.AdminID, entry.AdminUsername, entry.Action, nullableInt64(entry.EventID), entry.EventIDs, query, details)
	return err
}

func scanEvents(rows pgx.Rows) ([]Event, error) {
	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.ServerID, &e.ServerName, &e.EventType, &e.Severity,
			&e.PlayerSource, &e.PlayerName, &e.License, &e.Discord, &e.Steam, &e.CitizenID,
			&e.Resource, &e.Message, &e.CoordsX, &e.CoordsY, &e.CoordsZ,
			&e.Metadata, &e.OccurredAt, &e.CreatedAt, &e.Review.Status, &e.Review.Note,
			&e.Review.ArchivedAt, &e.Review.ArchivedBy, &e.Review.UpdatedBy, &e.Review.UpdatedAt,
			&e.Review.ArchivedByName, &e.Review.UpdatedByName); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func nullableInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func cleanEventIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func normalizeQuery(q Query) Query {
	q.Limit, q.Offset = normalizeLimitOffset(q.Limit, q.Offset)
	q.ReviewStatus = NormalizeReviewStatus(q.ReviewStatus)
	q.ArchiveMode = NormalizeArchiveMode(q.ArchiveMode)
	return q
}

func normalizeLimitOffset(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 5000 {
		limit = 5000
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func NormalizeReviewStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case ReviewStatusNormal:
		return ReviewStatusNormal
	case ReviewStatusSuspicious:
		return ReviewStatusSuspicious
	case ReviewStatusViolation:
		return ReviewStatusViolation
	default:
		return ""
	}
}

func NormalizeArchiveMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ArchiveInclude:
		return ArchiveInclude
	case ArchiveArchivedOnly:
		return ArchiveArchivedOnly
	default:
		return ArchiveActiveOnly
	}
}

func queryWhere(q Query, args *[]any) []string {
	where := []string{"1=1"}
	add := func(clause string, value any) {
		*args = append(*args, value)
		where = append(where, fmt.Sprintf(clause, len(*args)))
	}

	if q.ServerID > 0 {
		add("e.server_id = $%d", q.ServerID)
	}
	if q.EventType != "" {
		add("e.event_type = $%d", q.EventType)
	}
	if q.Severity != "" {
		add("e.severity = $%d", q.Severity)
	}
	if q.Resource != "" {
		add("e.resource ILIKE $%d", "%"+q.Resource+"%")
	}
	if q.Keyword != "" {
		add(`(
			e.message ILIKE $%[1]d OR e.player_name ILIKE $%[1]d OR
			e.license ILIKE $%[1]d OR e.discord ILIKE $%[1]d OR
			e.steam ILIKE $%[1]d OR e.citizenid ILIKE $%[1]d OR
			e.resource ILIKE $%[1]d OR e.metadata::text ILIKE $%[1]d
		)`, "%"+q.Keyword+"%")
	}
	if q.Player != "" {
		*args = append(*args, "%"+q.Player+"%", q.Player)
		where = append(where, playerWhereClause(len(*args)-1, len(*args)))
	}
	if q.Metadata != "" && json.Valid([]byte(q.Metadata)) {
		add("e.metadata @> $%d::jsonb", q.Metadata)
	}
	if q.ReviewStatus != "" {
		add("coalesce((SELECT rs.status FROM log_event_reviews rs WHERE rs.event_id = e.id), 'normal') = $%d", q.ReviewStatus)
	}
	switch q.ArchiveMode {
	case ArchiveInclude:
	case ArchiveArchivedOnly:
		where = append(where, "EXISTS (SELECT 1 FROM log_event_reviews ar WHERE ar.event_id = e.id AND ar.archived_at IS NOT NULL)")
	default:
		where = append(where, "NOT EXISTS (SELECT 1 FROM log_event_reviews ar WHERE ar.event_id = e.id AND ar.archived_at IS NOT NULL)")
	}
	if q.Since != nil {
		add("e.occurred_at >= $%d", *q.Since)
	}
	if q.Until != nil {
		add("e.occurred_at <= $%d", *q.Until)
	}
	if q.WithCoords {
		where = append(where, "e.coords_x IS NOT NULL AND e.coords_y IS NOT NULL")
	}
	return where
}

func playerWhereClause(namePatternArg, identifierArg int) string {
	return fmt.Sprintf(
		"(e.player_name ILIKE $%d OR e.license = $%d OR e.discord = $%d OR e.steam = $%d OR e.citizenid = $%d)",
		namePatternArg,
		identifierArg,
		identifierArg,
		identifierArg,
		identifierArg,
	)
}

func NewPage(limit, offset int, total int64) Page {
	limit, offset = normalizeLimitOffset(limit, offset)
	if total < 0 {
		total = 0
	}
	totalPages := 0
	current := 0
	if total > 0 {
		totalPages = int((total + int64(limit) - 1) / int64(limit))
		current = (offset / limit) + 1
		if current > totalPages {
			current = totalPages
			offset = (totalPages - 1) * limit
		}
	}
	page := Page{
		Limit:      limit,
		Offset:     offset,
		Total:      total,
		Current:    current,
		TotalPages: totalPages,
		PrevOffset: offset - limit,
		NextOffset: offset + limit,
		HasPrev:    offset > 0,
		HasNext:    int64(offset+limit) < total,
	}
	if page.PrevOffset < 0 {
		page.PrevOffset = 0
	}
	if total > 0 {
		page.From = int64(offset) + 1
		page.To = int64(offset + limit)
		if page.To > total {
			page.To = total
		}
	}
	page.Items = pageItems(current, totalPages, limit)
	return page
}

func pageItems(current, totalPages, limit int) []PageItem {
	if current <= 0 || totalPages <= 0 {
		return nil
	}
	if limit <= 0 {
		limit = 100
	}
	if totalPages <= 7 {
		items := make([]PageItem, 0, totalPages)
		for number := 1; number <= totalPages; number++ {
			items = append(items, pageItem(number, current, limit))
		}
		return items
	}

	pages := []int{1, totalPages}
	for number := current - 2; number <= current+2; number++ {
		if number > 1 && number < totalPages {
			pages = append(pages, number)
		}
	}
	pages = sortedUniqueInts(pages)

	items := make([]PageItem, 0, len(pages)+2)
	prev := 0
	for _, number := range pages {
		if prev > 0 && number-prev > 1 {
			items = append(items, PageItem{Gap: true})
		}
		items = append(items, pageItem(number, current, limit))
		prev = number
	}
	return items
}

func pageItem(number, current, limit int) PageItem {
	return PageItem{
		Number: number,
		Offset: (number - 1) * limit,
		Active: number == current,
	}
}

func sortedUniqueInts(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		inserted := false
		for i, existing := range out {
			if value == existing {
				inserted = true
				break
			}
			if value < existing {
				out = append(out, 0)
				copy(out[i+1:], out[i:])
				out[i] = value
				inserted = true
				break
			}
		}
		if !inserted {
			out = append(out, value)
		}
	}
	return out
}
