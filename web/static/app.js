const state = {
  paused: false,
  source: null,
  currentLogEvent: null,
  activeJSONView: "metadata",
  aiJSONMethods: null,
};

function appTimeZone() {
  return document.body?.dataset.timeZone || "Asia/Shanghai";
}

function t(key, replacements = {}) {
  const catalog = window.VFL_I18N && typeof window.VFL_I18N === "object" ? window.VFL_I18N : {};
  let value = catalog[key] || key;
  Object.entries(replacements).forEach(([name, replacement]) => {
    value = value.replaceAll(`{${name}}`, String(replacement));
  });
  return value;
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function icon(name, className = "icon") {
  return `<svg class="${className}" aria-hidden="true"><use href="#icon-${name}"></use></svg>`;
}

function severityClass(severity) {
  if (severity === "error") return "error";
  if (severity === "warning") return "warning";
  if (severity === "success") return "success";
  return "info";
}

function review(event) {
  return event && typeof event.review === "object" && event.review !== null ? event.review : {};
}

function reviewStatus(event) {
  return review(event).status || "normal";
}

function reviewStatusLabel(status) {
  if (status === "suspicious") return t("review.status.suspicious");
  if (status === "violation") return t("review.status.violation");
  return t("review.status.normal");
}

function reviewStatusClass(status) {
  if (status === "suspicious") return "warning";
  if (status === "violation") return "error";
  return "success";
}

function formatTime(value) {
  if (!value) return "";
  const d = new Date(value);
  return d.toLocaleTimeString([], { hourCycle: "h23", timeZone: appTimeZone() });
}

function fullTime(value) {
  if (!value) return "";
  const d = new Date(value);
  return d.toLocaleString([], { hourCycle: "h23", timeZone: appTimeZone() });
}

function timeParts(date) {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: appTimeZone(),
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).formatToParts(date);
  const values = {};
  parts.forEach((part) => {
    if (part.type !== "literal") values[part.type] = part.value;
  });
  return values;
}

function compactID(value) {
  const text = String(value || "");
  if (text.length <= 18) return text;
  return `${text.slice(0, 10)}...${text.slice(-5)}`;
}

function playerIdentifier(event) {
  return event.license || event.citizenid || event.discord || event.steam || event.player_name || "";
}

function metadata(event) {
  return event && typeof event.metadata === "object" && event.metadata !== null ? event.metadata : {};
}

function metaValue(event, ...keys) {
  const meta = metadata(event);
  for (const key of keys) {
    const value = meta[key];
    if (value !== undefined && value !== null && value !== "") return String(value);
  }
  return "";
}

function actorName(event) {
  const character = metaValue(event, "character_name", "characterName", "char_name", "charName");
  const player = event.player_name || "";
  if (character && player && character !== player) return `${character} / ${player}`;
  return character || player || "system";
}

function eventSummary(event) {
  const meta = metadata(event);
  if (event.event_type === "money_change") {
    return [
      metaValue(event, "money_type", "account"),
      metaValue(event, "operation"),
      metaValue(event, "amount"),
      metaValue(event, "balance") ? `balance ${metaValue(event, "balance")}` : "",
      metaValue(event, "reason"),
    ].filter(Boolean).join(" · ") || event.message || "";
  }
  if (event.event_type === "inventory_diff" && meta.changes) {
    const changes = normalizeInventoryChanges(meta.changes);
    const parts = changes.slice(0, 4).map(({ change }) => {
      return `${change.label || change.name || "item"} ${formatDelta(change.delta)}`;
    });
    if (changes.length > 4) parts.push(`+${changes.length - 4} more`);
    return parts.join(", ") || event.message || "";
  }
  if (event.event_type === "inventory_add" || event.event_type === "inventory_remove") {
    return [
      metaValue(event, "item", "name", "itemName"),
      metaValue(event, "count", "amount") ? `x${metaValue(event, "count", "amount")}` : "",
      metaValue(event, "reason"),
    ].filter(Boolean).join(" · ") || event.message || "";
  }
  return event.message || "";
}

function metaChips(event) {
  const keys = ["job", "gang", "money_type", "operation", "reason", "context_text", "plate", "weapon"];
  const chips = keys.map((key) => {
    const value = metaValue(event, key);
    if (!value) return "";
    const clipped = value.length > 36 ? `${value.slice(0, 36)}...` : value;
    return `${key}=${clipped}`;
  }).filter(Boolean);
  const changes = metaValue(event, "change_count");
  if (changes) chips.push(`changes=${changes}`);
  return chips.slice(0, 4);
}

function field(label, value) {
  if (value === undefined || value === null || value === "") return "";
  return `<div><span>${escapeHTML(label)}</span><strong>${escapeHTML(value)}</strong></div>`;
}

function isPlainObject(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function normalizeInventoryChanges(changes) {
  if (Array.isArray(changes)) {
    return changes
      .map((change, index) => ({ slot: String(change?.slot || index + 1), change }))
      .filter(({ change }) => isPlainObject(change));
  }
  if (!isPlainObject(changes)) return [];
  return Object.entries(changes)
    .filter(([, change]) => isPlainObject(change))
    .sort(([left], [right]) => {
      const leftNum = Number(left);
      const rightNum = Number(right);
      if (Number.isFinite(leftNum) && Number.isFinite(rightNum)) return leftNum - rightNum;
      return left.localeCompare(right);
    })
    .map(([slot, change]) => ({ slot, change }));
}

function formatCount(value) {
  if (value === undefined || value === null || value === "") return "-";
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return String(value);
  return Number.isInteger(numeric) ? String(numeric) : numeric.toFixed(3).replace(/\.?0+$/, "");
}

function formatDelta(value) {
  const numeric = Number(value || 0);
  if (!Number.isFinite(numeric)) return String(value ?? "");
  const formatted = formatCount(numeric);
  return numeric > 0 ? `+${formatted}` : formatted;
}

function deltaClass(value) {
  const numeric = Number(value || 0);
  if (numeric > 0) return "positive";
  if (numeric < 0) return "negative";
  return "neutral";
}

function parseEmbeddedJSON(value) {
  if (typeof value !== "string") return { parsed: false, value };
  const trimmed = value.trim();
  if (trimmed.length < 2 || !["{", "["].includes(trimmed[0])) return { parsed: false, value };
  try {
    return { parsed: true, value: JSON.parse(trimmed) };
  } catch {
    return { parsed: false, value };
  }
}

function expandEmbeddedJSON(value, depth = 0) {
  if (depth > 5) return { value, expanded: false };
  const embedded = parseEmbeddedJSON(value);
  if (embedded.parsed) {
    const nested = expandEmbeddedJSON(embedded.value, depth + 1);
    return { value: nested.value, expanded: true };
  }
  if (Array.isArray(value)) {
    let expanded = false;
    const items = value.map((item) => {
      const nested = expandEmbeddedJSON(item, depth + 1);
      expanded = expanded || nested.expanded;
      return nested.value;
    });
    return { value: items, expanded };
  }
  if (isPlainObject(value)) {
    let expanded = false;
    const object = {};
    Object.entries(value).forEach(([key, item]) => {
      const nested = expandEmbeddedJSON(item, depth + 1);
      expanded = expanded || nested.expanded;
      object[key] = nested.value;
    });
    return { value: object, expanded };
  }
  return { value, expanded: false };
}

function highlightedJSON(value) {
  const json = JSON.stringify(value, null, 2) || "null";
  const tokenPattern = /("(?:\\.|[^"\\])*"(?=\s*:)|"(?:\\.|[^"\\])*"|\btrue\b|\bfalse\b|\bnull\b|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g;
  return json.replace(tokenPattern, (token, offset) => {
    let cls = "json-number";
    if (token === "true" || token === "false") cls = "json-boolean";
    else if (token === "null") cls = "json-null";
    else if (token.startsWith('"')) cls = json.slice(offset + token.length).match(/^\s*:/) ? "json-key" : "json-string";
    return `<span class="${cls}">${escapeHTML(token)}</span>`;
  });
}

function aiJSONMethods() {
  if (Array.isArray(state.aiJSONMethods)) return state.aiJSONMethods;
  const node = document.querySelector("#ai-json-methods");
  if (!node) {
    state.aiJSONMethods = [];
    return state.aiJSONMethods;
  }
  const raw = node instanceof HTMLTemplateElement
    ? node.content.textContent
    : node.textContent;
  try {
    const parsed = JSON.parse(raw || "[]");
    state.aiJSONMethods = Array.isArray(parsed) ? parsed : [];
  } catch {
    state.aiJSONMethods = [];
  }
  return state.aiJSONMethods;
}

function pathTokens(path) {
  return String(path || "")
    .replace(/\[(\d*)\]/g, ".$1")
    .split(".")
    .map((part) => part.trim())
    .filter(Boolean);
}

function valueAtPath(root, path) {
  if (!path) return root;
  let current = root;
  for (const token of pathTokens(path)) {
    if (current === undefined || current === null) return undefined;
    if (Array.isArray(current)) {
      const index = Number(token);
      if (!Number.isInteger(index)) return undefined;
      current = current[index];
    } else {
      current = current[token];
    }
  }
  return current;
}

function stripPathPrefix(path, prefix) {
  const text = String(path || "").trim();
  const normalized = text.toLowerCase();
  const lowerPrefix = prefix.toLowerCase();
  if (normalized === lowerPrefix.slice(0, -1)) return "";
  return normalized.startsWith(lowerPrefix) ? text.slice(prefix.length) : null;
}

function specPathCandidates(method, path) {
  const source = method?.source === "event" ? "event" : "metadata";
  const raw = String(path || "").trim().replace(/^\$\./, "");
  const candidates = [raw];
  const prefixes = source === "event"
    ? ["event."]
    : ["metadata.", "event.metadata."];
  prefixes.forEach((prefix) => {
    const stripped = stripPathPrefix(raw, prefix);
    if (stripped !== null) candidates.push(stripped);
  });
  return [...new Set(candidates)];
}

function valueAtSpecPath(method, event, path) {
  const sourceRoot = methodSource(method, event);
  for (const candidate of specPathCandidates(method, path)) {
    const value = valueAtPath(sourceRoot, candidate);
    if (value !== undefined) return value;
  }
  const raw = String(path || "").trim().replace(/^\$\./, "");
  for (const candidate of [raw, stripPathPrefix(raw, "event."), `metadata.${raw}`].filter((item) => item !== null)) {
    const value = valueAtPath(event, candidate);
    if (value !== undefined) return value;
  }
  return undefined;
}

function methodScopeScore(method, event) {
  if (!method || !event) return 0;
  let score = 1;
  if (method.event_type) {
    if (String(method.event_type).toLowerCase() !== String(event.event_type || "").toLowerCase()) return 0;
    score += 4;
  }
  if (method.resource) {
    if (String(event.resource || "").toLowerCase() !== String(method.resource).toLowerCase()) return 0;
    score += 3;
  }
  return score;
}

function matchingAIJSONMethods(event) {
  return aiJSONMethods()
    .filter((method) => method && method.active !== false && methodScopeScore(method, event) > 0)
    .sort((left, right) => methodScopeScore(right, event) - methodScopeScore(left, event) || String(left.name || "").localeCompare(String(right.name || "")));
}

function methodSource(method, event) {
  return method?.source === "event" ? event : metadata(event);
}

function normalizeFormat(format) {
  return String(format || "text").trim().toLowerCase().replaceAll("-", "_");
}

function firstPresent(...values) {
  return values.find((value) => value !== undefined && value !== null && value !== "");
}

function hasDisplayValue(value) {
  if (value === undefined || value === null || value === "") return false;
  if (Array.isArray(value)) return value.length > 0;
  return true;
}

function numberValue(value) {
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric : null;
}

function formatDateValue(value, options = {}) {
  if (!value) return "";
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  const mode = normalizeFormat(options.dateMode || options.format);
  if (mode === "date") {
    return date.toLocaleDateString([], { timeZone: appTimeZone() });
  }
  if (mode === "clock") {
    return date.toLocaleTimeString([], { hourCycle: "h23", timeZone: appTimeZone() });
  }
  if (mode === "iso") {
    return date.toISOString();
  }
  return date.toLocaleString([], { hourCycle: "h23", timeZone: appTimeZone() });
}

function formatDuration(value, unit = "ms") {
  const numeric = numberValue(value);
  if (numeric === null) return String(value ?? "");
  let seconds = unit === "s" || unit === "sec" || unit === "seconds" ? numeric : numeric / 1000;
  const sign = seconds < 0 ? "-" : "";
  seconds = Math.abs(seconds);
  if (seconds < 60) return `${sign}${formatCount(seconds)}s`;
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remainder = Math.floor(seconds % 60);
  if (hours) return `${sign}${hours}h ${minutes}m ${remainder}s`;
  return `${sign}${minutes}m ${remainder}s`;
}

function formatCoordinates(value) {
  if (Array.isArray(value)) {
    return value.map((item) => formatCount(item)).join(", ");
  }
  if (isPlainObject(value)) {
    return ["x", "y", "z"].map((key) => value[key]).filter((item) => item !== undefined && item !== null).map((item) => formatCount(item)).join(", ");
  }
  return String(value ?? "");
}

function mappedValue(value, valueMap) {
  if (!valueMap || typeof valueMap !== "object") return value;
  const key = String(value);
  return Object.prototype.hasOwnProperty.call(valueMap, key) ? valueMap[key] : value;
}

function displayValue(value, format = "text", options = {}) {
  if (!hasDisplayValue(value)) return "";
  const formatName = normalizeFormat(format);
  const mapped = mappedValue(value, options.value_map || options.valueMap);
  const prefix = options.prefix || "";
  const suffix = options.suffix || "";
  let output = "";

  if (formatName === "json" || formatName === "pretty_json") {
    output = JSON.stringify(mapped, null, 2);
  } else if (["time", "datetime", "date", "clock", "iso"].includes(formatName)) {
    output = formatDateValue(mapped, { ...options, format: formatName });
  } else if (formatName === "delta") {
    output = formatDelta(mapped);
  } else if (formatName === "number" || formatName === "count") {
    const numeric = numberValue(mapped);
    const precision = Number(options.precision);
    output = numeric !== null && Number.isInteger(precision) && precision >= 0
      ? numeric.toFixed(Math.min(precision, 8)).replace(/\.?0+$/, "")
      : formatCount(mapped);
  } else if (formatName === "currency") {
    const numeric = numberValue(mapped);
    if (numeric === null) {
      output = String(mapped);
    } else if (options.currency) {
      try {
        output = new Intl.NumberFormat([], { style: "currency", currency: String(options.currency) }).format(numeric);
      } catch {
        output = formatCount(numeric);
      }
    } else {
      output = formatCount(numeric);
    }
  } else if (formatName === "percent") {
    const numeric = numberValue(mapped);
    output = numeric === null ? String(mapped) : `${formatCount(Math.abs(numeric) <= 1 ? numeric * 100 : numeric)}%`;
  } else if (formatName === "duration" || formatName === "duration_ms") {
    output = formatDuration(mapped, options.unit || "ms");
  } else if (formatName === "duration_s") {
    output = formatDuration(mapped, "s");
  } else if (formatName === "coords" || formatName === "coordinates") {
    output = formatCoordinates(mapped);
  } else if (formatName === "boolean" || formatName === "bool" || formatName === "yes_no") {
    output = mapped === true || mapped === "true" || mapped === 1 || mapped === "1" ? t("boolean.yes") : t("boolean.no");
  } else if (formatName === "list" || formatName === "tags") {
    output = Array.isArray(mapped) ? mapped.map((item) => displayValue(item, "text")).filter(Boolean).join(options.separator || ", ") : String(mapped);
  } else if (typeof mapped === "object") {
    output = JSON.stringify(mapped);
  } else {
    output = String(mapped);
  }

  if (!output) return "";
  const maxLength = Number(options.max_length || options.maxLength);
  if (Number.isInteger(maxLength) && maxLength > 3 && output.length > maxLength) {
    output = `${output.slice(0, maxLength - 3)}...`;
  }
  return `${prefix}${output}${suffix}`;
}

function toneFromValue(value, format = "text", spec = {}) {
  const allowed = ["info", "success", "warning", "error", "muted"];
  const toneMap = spec.tone_map || spec.toneMap;
  if (toneMap && typeof toneMap === "object") {
    const mapped = toneMap[String(value)];
    if (allowed.includes(String(mapped).toLowerCase())) return String(mapped).toLowerCase();
  }
  const explicit = String(spec.tone || "").toLowerCase();
  if (allowed.includes(explicit)) return explicit;
  if (explicit !== "auto") return "info";
  const formatName = normalizeFormat(format);
  const text = String(value ?? "").toLowerCase();
  if (["error", "violation", "failed", "fail", "denied", "ban"].includes(text)) return "error";
  if (["warning", "warn", "suspicious"].includes(text)) return "warning";
  if (["success", "ok", "normal", "allowed"].includes(text)) return "success";
  if (formatName === "delta") {
    const numeric = numberValue(value);
    if (numeric !== null && numeric < 0) return "error";
    if (numeric !== null && numeric > 0) return "success";
  }
  return "info";
}

function fieldPaths(spec = {}) {
  return [spec.path, ...(Array.isArray(spec.paths) ? spec.paths : []), ...(Array.isArray(spec.fallback_paths) ? spec.fallback_paths : [])]
    .filter((path) => path !== undefined && path !== null);
}

function specRawValue(method, event, spec = {}, localRoot) {
  if (Object.prototype.hasOwnProperty.call(spec, "value")) return spec.value;
  const paths = fieldPaths(spec);
  for (const path of paths) {
    const value = localRoot === undefined ? valueAtSpecPath(method, event, path) : valueAtPath(localRoot, path);
    if (hasDisplayValue(value) || spec.show_empty === true) return value;
  }
  return undefined;
}

function renderTemplateValue(template, method, event, localRoot) {
  return String(template || "").replace(/\{\{\s*([^{}]+?)\s*\}\}|\{\s*([^{}]+?)\s*\}/g, (_match, doublePath, singlePath) => {
    const rawPath = (doublePath || singlePath || "").trim();
    if (!rawPath) return "";
    const value = localRoot === undefined ? valueAtSpecPath(method, event, rawPath) : valueAtPath(localRoot, rawPath);
    return displayValue(value, "text");
  });
}

function specDisplayValue(method, event, spec = {}, localRoot) {
  if (spec.template) {
    return renderTemplateValue(spec.template, method, event, localRoot);
  }
  const value = specRawValue(method, event, spec, localRoot);
  if (!hasDisplayValue(value)) return firstPresent(spec.empty, spec.default, "");
  return displayValue(value, spec.format || "text", spec);
}

function rowsFromValue(value) {
  if (Array.isArray(value)) {
    return value.map((item, index) => isPlainObject(item) ? { index: index + 1, ...item } : { index: index + 1, value: item });
  }
  if (isPlainObject(value)) {
    return Object.entries(value).map(([key, item]) => isPlainObject(item) ? { key, ...item } : { key, value: item });
  }
  return [];
}

function limitedRows(rows, spec = {}) {
  const limit = Number(spec.limit || spec.max_rows || spec.maxRows || 80);
  const max = Number.isInteger(limit) && limit > 0 ? Math.min(limit, 500) : 80;
  return rows.slice(0, max);
}

function renderAIJSONBadge(method, event, badge, localRoot) {
  const raw = specRawValue(method, event, badge, localRoot);
  const value = specDisplayValue(method, event, badge, localRoot);
  if (!value && badge.show_empty !== true) return "";
  const tone = toneFromValue(raw, badge.format || "text", badge);
  const label = badge.label ? `${badge.label}=` : "";
  return `<span class="chip ${escapeHTML(tone)}">${escapeHTML(label)}${escapeHTML(value || badge.empty || "-")}</span>`;
}

function renderAIJSONField(method, event, fieldSpec, localRoot) {
  const raw = specRawValue(method, event, fieldSpec, localRoot);
  const value = specDisplayValue(method, event, fieldSpec, localRoot);
  if (!value && fieldSpec.show_empty !== true) return "";
  const subtitle = firstPresent(
    fieldSpec.subtitle,
    fieldSpec.sub_path ? displayValue(localRoot === undefined ? valueAtSpecPath(method, event, fieldSpec.sub_path) : valueAtPath(localRoot, fieldSpec.sub_path), fieldSpec.sub_format || "text", fieldSpec) : "",
  );
  const tone = fieldSpec.tone ? ` tone-${escapeHTML(toneFromValue(raw, fieldSpec.format || "text", fieldSpec))}` : "";
  const width = fieldSpec.wide === true || fieldSpec.span === "wide" ? " wide" : fieldSpec.full === true || fieldSpec.span === "full" ? " full" : "";
  const valueClass = normalizeFormat(fieldSpec.format) === "json" || normalizeFormat(fieldSpec.format) === "pretty_json" ? " ai-json-field-pre" : "";
  return `
    <div class="ai-json-field${width}${tone}">
      <span>${escapeHTML(fieldSpec.label || fieldSpec.path || "field")}</span>
      <strong class="ai-json-field-value${valueClass}">${escapeHTML(value || fieldSpec.empty || "-")}</strong>
      ${subtitle ? `<em class="ai-json-field-subtitle">${escapeHTML(subtitle)}</em>` : ""}
    </div>
  `;
}

function renderAIJSONFieldGrid(method, event, fields, localRoot) {
  const html = (Array.isArray(fields) ? fields : [])
    .map((fieldSpec) => renderAIJSONField(method, event, fieldSpec, localRoot))
    .filter(Boolean)
    .join("");
  return html ? `<div class="compact-detail-grid ai-json-fields">${html}</div>` : "";
}

function renderAIJSONMetrics(method, event, metrics) {
  const html = (Array.isArray(metrics) ? metrics : [])
    .map((metric) => {
      const raw = specRawValue(method, event, metric);
      const value = specDisplayValue(method, event, metric);
      if (!value && metric.show_empty !== true) return "";
      const tone = toneFromValue(raw, metric.format || "text", { ...metric, tone: metric.tone || "auto" });
      return `
        <div class="ai-json-metric tone-${escapeHTML(tone)}">
          <span>${escapeHTML(metric.label || metric.path || "metric")}</span>
          <strong>${escapeHTML(value || metric.empty || "-")}</strong>
        </div>
      `;
    })
    .filter(Boolean)
    .join("");
  return html ? `<div class="ai-json-metrics">${html}</div>` : "";
}

function renderAIJSONSections(method, event, sections) {
  return (Array.isArray(sections) ? sections : [])
    .map((section) => {
      const fields = renderAIJSONFieldGrid(method, event, section.fields);
      const badges = (Array.isArray(section.badges) ? section.badges : [])
        .map((badge) => renderAIJSONBadge(method, event, badge))
        .filter(Boolean)
        .join("");
      if (!fields && !badges) return "";
      return `
        <section class="ai-json-section">
          <div class="ai-json-section-title">
            <h4>${escapeHTML(section.title || t("json.detail"))}</h4>
            ${badges ? `<div class="metadata-pills">${badges}</div>` : ""}
          </div>
          ${fields}
        </section>
      `;
    })
    .filter(Boolean)
    .join("");
}

function renderAIJSONLists(method, event, lists) {
  return (Array.isArray(lists) ? lists : [])
    .map((listSpec) => {
      const rows = limitedRows(rowsFromValue(valueAtSpecPath(method, event, listSpec.path)), listSpec);
      if (!rows.length) return "";
      const items = rows.map((row, index) => {
        const titleSpec = { path: listSpec.title_path || listSpec.item_title_path || "label", fallback_paths: [listSpec.label_path || "name", "key", "value"], format: listSpec.title_format || "text" };
        const subtitlePath = listSpec.subtitle_path || listSpec.item_subtitle_path || "";
        const subtitleSpec = { path: subtitlePath, format: listSpec.subtitle_format || "text" };
        const title = specDisplayValue(method, event, titleSpec, row) || `#${index + 1}`;
        const subtitle = subtitlePath && titleSpec.path !== subtitlePath ? specDisplayValue(method, event, subtitleSpec, row) : "";
        const badges = (Array.isArray(listSpec.badges) ? listSpec.badges : [])
          .map((badge) => renderAIJSONBadge(method, event, badge, row))
          .filter(Boolean)
          .join("");
        const fields = renderAIJSONFieldGrid(method, event, listSpec.fields, row);
        return `
          <article class="ai-json-list-item">
            <div class="ai-json-list-item-head">
              <div>
                <strong>${escapeHTML(title)}</strong>
                ${subtitle ? `<span>${escapeHTML(subtitle)}</span>` : ""}
              </div>
              ${badges ? `<div class="metadata-pills">${badges}</div>` : ""}
            </div>
            ${fields}
          </article>
        `;
      }).join("");
      return `
        <section class="ai-json-list">
          <h4>${escapeHTML(listSpec.title || t("json.list"))}</h4>
          <div>${items}</div>
        </section>
      `;
    })
    .filter(Boolean)
    .join("");
}

function renderAIJSONTableCell(method, event, column, row) {
  const raw = specRawValue(method, event, column, row);
  const value = specDisplayValue(method, event, column, row);
  const subtitle = column.sub_path
    ? displayValue(valueAtPath(row, column.sub_path), column.sub_format || "text", column)
    : "";
  const formatName = normalizeFormat(column.format);
  const tone = column.tone ? ` tone-${escapeHTML(toneFromValue(raw, column.format || "text", column))}` : "";
  const align = ["right", "center"].includes(column.align) ? ` align-${column.align}` : "";
  if (formatName === "delta") {
    return `<td class="${align.trim()}"><span class="delta ${deltaClass(raw)}">${escapeHTML(value)}</span></td>`;
  }
  if (formatName === "json" || formatName === "pretty_json") {
    return `<td class="ai-json-table-json${align}"><pre>${escapeHTML(value)}</pre></td>`;
  }
  return `
    <td class="${`${tone}${align}`.trim()}">
      ${value ? `<strong>${escapeHTML(value)}</strong>` : `<span class="muted">${escapeHTML(column.empty || "-")}</span>`}
      ${subtitle ? `<span>${escapeHTML(subtitle)}</span>` : ""}
    </td>
  `;
}

function renderAIJSONTables(method, event, tables) {
  return (Array.isArray(tables) ? tables : [])
    .map((tableSpec) => {
      const items = limitedRows(rowsFromValue(valueAtSpecPath(method, event, tableSpec.path)), tableSpec);
      const columns = Array.isArray(tableSpec.columns) ? tableSpec.columns : [];
      if (!items.length || !columns.length) return "";
      const head = columns.map((column) => `<th>${escapeHTML(column.label || column.path || "")}</th>`).join("");
      const body = items.map((row) => {
        const cells = columns.map((column) => renderAIJSONTableCell(method, event, column, row)).join("");
        return `<tr>${cells}</tr>`;
      }).join("");
      const caption = tableSpec.description ? `<p>${escapeHTML(tableSpec.description)}</p>` : "";
      return `
        <div class="ai-json-table-wrap">
          <div class="ai-json-table-title">
            <h4>${escapeHTML(tableSpec.title || t("json.table"))}</h4>
            ${caption}
          </div>
          <table class="inventory-table ai-json-table"><thead><tr>${head}</tr></thead><tbody>${body}</tbody></table>
        </div>
      `;
    })
    .filter(Boolean)
    .join("");
}

function renderAIJSONBlocks(method, event, blocks) {
  return (Array.isArray(blocks) ? blocks : [])
    .map((block) => {
      const value = specRawValue(method, event, block);
      if (!hasDisplayValue(value) && block.show_empty !== true) return "";
      const display = expandEmbeddedJSON(value);
      return `
        <section class="ai-json-json-block">
          <h4>${escapeHTML(block.title || block.label || block.path || "JSON")}</h4>
          <pre class="detail-json">${highlightedJSON(display.value ?? {})}</pre>
        </section>
      `;
    })
    .filter(Boolean)
    .join("");
}

function renderAIJSONMethod(method, event) {
  if (!method || !event) return "";
  const spec = method.spec && typeof method.spec === "object" ? method.spec : {};
  const title = spec.title || method.name || "AI JSON";
  const summary = spec.summary_template
    ? renderTemplateValue(spec.summary_template, method, event)
    : spec.summary_path
      ? displayValue(valueAtSpecPath(method, event, spec.summary_path), "text", spec)
      : "";
  const badgeHTML = (Array.isArray(spec.badges) ? spec.badges : [])
    .map((badge) => renderAIJSONBadge(method, event, badge))
    .filter(Boolean)
    .join("");
  const metricHTML = renderAIJSONMetrics(method, event, spec.metrics);
  const fieldHTML = renderAIJSONFieldGrid(method, event, spec.fields);
  const sectionHTML = renderAIJSONSections(method, event, spec.sections);
  const listHTML = renderAIJSONLists(method, event, spec.lists);
  const tableHTML = renderAIJSONTables(method, event, spec.tables);
  const blockHTML = renderAIJSONBlocks(method, event, spec.json_blocks || spec.jsonBlocks);

  if (!badgeHTML && !metricHTML && !fieldHTML && !sectionHTML && !listHTML && !tableHTML && !blockHTML && !summary) return "";
  return `
    <section class="ai-json-rendered">
      <div class="insight-head">
        <div>
          <h3>${escapeHTML(title)}</h3>
          <p>${escapeHTML(spec.description || method.description || t("ai_json.saved_display_method"))}</p>
        </div>
        <div class="metadata-pills">${badgeHTML}</div>
      </div>
      ${summary ? `<div class="ai-json-summary">${escapeHTML(summary)}</div>` : ""}
      ${metricHTML}
      ${fieldHTML}
      ${sectionHTML}
      ${listHTML}
      ${tableHTML}
      ${blockHTML}
    </section>
  `;
}

function renderAIJSONInsight(event) {
  const method = matchingAIJSONMethods(event)[0];
  return method ? renderAIJSONMethod(method, event) : "";
}

function populateAIJSONSelector(event) {
  const select = document.querySelector("[data-ai-json-method]");
  const target = document.querySelector("[data-ai-json-render]");
  if (!select) return;
  const methods = matchingAIJSONMethods(event);
  select.innerHTML = `<option value="">${escapeHTML(t("ai_json.method_label"))}</option>` + methods.map((method) => `<option value="${escapeHTML(method.id)}">${escapeHTML(method.name || `#${method.id}`)}</option>`).join("");
  select.disabled = methods.length === 0;
  if (!methods.length) {
    if (target) target.innerHTML = "";
    return;
  }
  select.value = String(methods[0].id);
  if (target) target.innerHTML = renderAIJSONMethod(methods[0], event);
}

function renderSelectedAIJSONMethod() {
  const select = document.querySelector("[data-ai-json-method]");
  const target = document.querySelector("[data-ai-json-render]");
  if (!select || !target || !state.currentLogEvent) return;
  const method = aiJSONMethods().find((item) => String(item.id) === String(select.value));
  target.innerHTML = method ? renderAIJSONMethod(method, state.currentLogEvent) : "";
}

function fullAIJSONSpecTemplate() {
  return {
    title: t("ai_json.default.title"),
    description: t("ai_json.template.description"),
    summary_template: "{action} {item} {amount}",
    badges: [
      { label: t("detail.action"), path: "action", format: "text", tone: "info" },
      { label: t("common.status"), path: "status", format: "text", tone: "auto" },
    ],
    metrics: [
      { label: t("ai_json.template.amount"), path: "amount", format: "number" },
      { label: t("ai_json.template.delta"), path: "delta", format: "delta", tone: "auto" },
    ],
    fields: [
      { label: t("common.player"), path: "character_name", paths: ["player_name"], format: "text" },
      { label: t("detail.reason"), path: "reason", format: "text", span: "wide" },
      { label: t("detail.coords"), path: "coords", format: "coords", span: "wide" },
    ],
    sections: [
      {
        title: t("ai_json.template.context"),
        fields: [
          { label: t("detail.job"), path: "job", format: "text" },
          { label: t("detail.gang"), path: "gang", format: "text" },
          { label: "Resource", path: "resource", format: "text" },
        ],
      },
    ],
    lists: [
      {
        title: t("ai_json.template.change_list"),
        path: "changes",
        title_path: "label",
        subtitle_path: "name",
        badges: [{ label: "delta", path: "delta", format: "delta", tone: "auto" }],
        fields: [
          { label: "Before", path: "before", format: "number" },
          { label: "After", path: "after", format: "number" },
          { label: "Metadata", path: "metadata", format: "json", span: "full" },
        ],
        limit: 20,
      },
    ],
    tables: [
      {
        title: t("ai_json.template.detail_table"),
        path: "changes",
        limit: 80,
        columns: [
          { label: "Key", path: "key", format: "text" },
          { label: t("ai_json.name"), path: "label", sub_path: "name", format: "text" },
          { label: "Before", path: "before", format: "number", align: "right" },
          { label: "After", path: "after", format: "number", align: "right" },
          { label: "Delta", path: "delta", format: "delta", align: "right" },
        ],
      },
    ],
    json_blocks: [
      { title: t("json.raw_metadata"), path: "" },
    ],
  };
}

function clippedText(value, max = 96) {
  const text = String(value ?? "");
  if (text.length <= max) return text;
  return `${text.slice(0, max - 3)}...`;
}

function compactMetaValue(value) {
  if (value === undefined || value === null || value === "") return "";
  if (typeof value === "string") {
    const embedded = parseEmbeddedJSON(value);
    if (embedded.parsed) {
      return Array.isArray(embedded.value) ? `JSON array[${embedded.value.length}]` : "JSON object";
    }
    if (value.startsWith("data:image/")) return "image data";
    return clippedText(value);
  }
  if (Array.isArray(value)) return value.length ? `array[${value.length}]` : "empty";
  if (isPlainObject(value)) return `object{${Object.keys(value).length}}`;
  return clippedText(value);
}

function metadataPills(value) {
  if (value === undefined || value === null) return `<span class="muted">empty</span>`;
  if (Array.isArray(value)) {
    if (!value.length) return `<span class="muted">empty</span>`;
    return value.slice(0, 5).map((item, index) => `<span>${index}=${escapeHTML(compactMetaValue(item))}</span>`).join("");
  }
  if (!isPlainObject(value)) return `<span>${escapeHTML(compactMetaValue(value))}</span>`;
  const entries = Object.entries(value).filter(([, item]) => item !== undefined && item !== null && item !== "");
  if (!entries.length) return `<span class="muted">empty</span>`;
  const visible = entries.slice(0, 6).map(([key, item]) => `<span>${escapeHTML(key)}=${escapeHTML(compactMetaValue(item))}</span>`);
  if (entries.length > visible.length) visible.push(`<span>+${entries.length - visible.length}</span>`);
  return visible.join("");
}

function renderInventoryInsight(event) {
  const meta = metadata(event);
  const changes = normalizeInventoryChanges(meta.changes);
  if (!changes.length) return "";
  const resource = String(event.resource || "").toLowerCase();
  const label = resource.includes("ox_inventory") ? t("insight.inventory_ox") : t("insight.inventory");
  const rows = changes.map(({ slot, change }) => {
    const title = change.label || change.name || "item";
    const subtitle = change.label && change.name && change.label !== change.name ? change.name : "";
    return `
      <tr>
        <td class="mono">${escapeHTML(slot)}</td>
        <td>
          <strong>${escapeHTML(title)}</strong>
          ${subtitle ? `<span>${escapeHTML(subtitle)}</span>` : ""}
        </td>
        <td class="mono">${escapeHTML(formatCount(change.before))}</td>
        <td class="mono">${escapeHTML(formatCount(change.after))}</td>
        <td><span class="delta ${deltaClass(change.delta)}">${escapeHTML(formatDelta(change.delta))}</span></td>
        <td><div class="metadata-pills">${metadataPills(change.metadata)}</div></td>
      </tr>
    `;
  }).join("");
  const context = [
    meta.action ? `action=${meta.action}` : "",
    meta.change_count ? `changes=${meta.change_count}` : changes.length ? `changes=${changes.length}` : "",
    meta.context_text || "",
    meta.character_name || "",
  ].filter(Boolean).map((item) => `<span>${escapeHTML(item)}</span>`).join("");

  return `
    <section class="log-insight inventory-insight">
      <div class="insight-head">
        <div>
          <h3>${escapeHTML(label)}</h3>
          <p>${escapeHTML(meta.job || "")}${meta.gang ? ` · ${escapeHTML(meta.gang)}` : ""}</p>
        </div>
        <div class="metadata-pills">${context}</div>
      </div>
      <div class="inventory-table-wrap">
        <table class="inventory-table">
          <thead>
            <tr><th>slot</th><th>item</th><th>before</th><th>after</th><th>delta</th><th>metadata</th></tr>
          </thead>
          <tbody>${rows}</tbody>
        </table>
      </div>
    </section>
  `;
}

function moneyInsightRows(event) {
  const rows = [
    [t("detail.account"), metaValue(event, "money_type", "account")],
    [t("detail.action"), metaValue(event, "operation")],
    [t("detail.amount"), metaValue(event, "amount")],
    [t("detail.balance"), metaValue(event, "balance")],
    [t("detail.reason"), metaValue(event, "reason")],
  ];
  return rows.filter(([, value]) => value !== "").map(([label, value]) => field(label, value)).join("");
}

function renderMoneyInsight(event) {
  if (event.event_type !== "money_change") return "";
  const rows = moneyInsightRows(event);
  if (!rows) return "";
  return `
    <section class="timeline-insight">
      <div class="insight-strip money">
        <strong>${escapeHTML(t("insight.money_change"))}</strong>
        <span>${escapeHTML(eventSummary(event))}</span>
      </div>
      <div class="compact-detail-grid">${rows}</div>
    </section>
  `;
}

function renderGenericInsight(event) {
  const meta = metadata(event);
  const entries = Object.entries(meta)
    .filter(([key, value]) => {
      if (["changes", "identifiers"].includes(key)) return false;
      return value !== undefined && value !== null && value !== "";
    })
    .slice(0, 8);
  if (!entries.length) return "";
  const pills = entries.map(([key, value]) => `<span>${escapeHTML(key)}=${escapeHTML(compactMetaValue(value))}</span>`).join("");
  return `
    <section class="timeline-insight">
      <div class="insight-strip">
        <strong>${escapeHTML(t("insight.summary"))}</strong>
        <div class="metadata-pills">${pills}</div>
      </div>
    </section>
  `;
}

function renderLogInsight(event) {
  if (!event) return "";
  return renderInventoryInsight(event) || renderMoneyInsight(event) || renderGenericInsight(event);
}

function renderLogJSON(event, view = state.activeJSONView || "metadata") {
  const pre = document.querySelector("[data-log-json]");
  const title = document.querySelector("[data-log-json-title]");
  const note = document.querySelector("[data-log-json-note]");
  if (!pre || !event) return;
  const raw = view === "event" ? event : metadata(event);
  const display = expandEmbeddedJSON(raw);
  pre.innerHTML = highlightedJSON(display.value);
  if (title) title.textContent = view === "event" ? t("ai_json.full_event_json") : "Metadata JSON";
  if (note) note.textContent = display.expanded ? t("json.expanded") : t("json.display_formatted");
  document.querySelectorAll("[data-json-view]").forEach((button) => {
    button.classList.toggle("active", button.dataset.jsonView === view);
  });
}

function hydrateAIJSONImportForm(event, source = "metadata") {
  const form = document.querySelector("[data-ai-json-import-form]");
  if (!form || !event) return;
  const selectedSource = source === "event" ? "event" : "metadata";
  const sample = selectedSource === "event" ? event : metadata(event);
  const fields = {
    source: form.querySelector("[data-ai-json-import-source]"),
    eventType: form.querySelector("[data-ai-json-import-event-type]"),
    resource: form.querySelector("[data-ai-json-import-resource]"),
    sample: form.querySelector("[data-ai-json-import-sample]"),
  };
  if (fields.source) fields.source.value = selectedSource;
  if (fields.eventType) fields.eventType.value = event.event_type || "";
  if (fields.resource) fields.resource.value = event.resource || "";
  if (fields.sample) fields.sample.value = JSON.stringify(sample, null, 2);
}

function hydrateReviewForm(event) {
  const panel = document.querySelector("[data-review-panel]");
  const form = document.querySelector("[data-review-form]");
  if (!panel || !form || !event || !event.id) return;
  const status = form.querySelector("[data-review-status]");
  const note = form.querySelector("[data-review-note-field]");
  const archive = form.querySelector("[data-review-archive]");
  const eventID = document.querySelector("[data-review-event-id]");
  const current = review(event);
  form.action = `/logs/${encodeURIComponent(event.id)}/review`;
  if (archive) archive.setAttribute("formaction", `/logs/${encodeURIComponent(event.id)}/archive`);
  if (status) status.value = current.status || "normal";
  if (note) note.value = current.note || "";
  if (eventID) eventID.textContent = `#${event.id}`;
  panel.hidden = false;
}

function parseTimelineEvent(node) {
  try {
    return JSON.parse(node.dataset.timelineEvent || "{}");
  } catch {
    return {};
  }
}

function hydrateTimelineCards() {
  document.querySelectorAll("[data-timeline-event]").forEach((card) => {
    const event = parseTimelineEvent(card);
    const insight = card.querySelector("[data-timeline-insight]");
    if (insight) insight.innerHTML = renderAIJSONInsight(event) || renderLogInsight(event);
    const pre = card.querySelector("[data-timeline-json]");
    const note = card.querySelector("[data-timeline-json-note]");
    if (pre) {
      const display = expandEmbeddedJSON(metadata(event));
      pre.innerHTML = highlightedJSON(display.value);
      if (note) note.textContent = display.expanded ? t("json.expanded") : t("json.formatted");
    }
  });
}

function syncReturnTargets() {
  document.querySelectorAll("[data-return-to]").forEach((input) => {
    input.value = `${window.location.pathname}${window.location.search}`;
  });
}

function selectedLogCount() {
  return document.querySelectorAll("[data-log-checkbox]:checked").length;
}

function updateSelectionCount() {
  const count = selectedLogCount();
  document.querySelectorAll("[data-selection-count]").forEach((node) => {
    node.textContent = `${count} ${t("logs.selected")}`;
  });
  const all = Array.from(document.querySelectorAll("[data-log-checkbox]"));
  document.querySelectorAll("[data-select-page-logs]").forEach((checkbox) => {
    checkbox.checked = all.length > 0 && all.every((item) => item.checked);
    checkbox.indeterminate = count > 0 && count < all.length;
  });
}

function bindBulkLogControls() {
  document.querySelectorAll("[data-select-page-logs]").forEach((checkbox) => {
    checkbox.addEventListener("change", () => {
      document.querySelectorAll("[data-log-checkbox]").forEach((item) => {
        item.checked = checkbox.checked;
      });
      updateSelectionCount();
    });
  });

  document.querySelectorAll("[data-log-checkbox]").forEach((checkbox) => {
    checkbox.addEventListener("change", updateSelectionCount);
  });

  document.querySelectorAll("[data-log-bulk-form]").forEach((form) => {
    form.addEventListener("submit", (event) => {
      const submitter = event.submitter;
      const action = submitter?.dataset.bulkAction || "";
      const count = selectedLogCount();
      if (count === 0 && !window.confirm(t("archive.no_selection_confirm"))) {
        event.preventDefault();
        return;
      }
      const target = count || t("archive.bulk_filter_target");
      if (action === "archive" && !window.confirm(t("archive.bulk_confirm", { target }))) {
        event.preventDefault();
      }
    });
  });

  updateSelectionCount();
}

function accountValueList(values) {
  if (!Array.isArray(values) || values.length === 0) return "";
  return values.filter(Boolean).join("\n");
}

function accountDetailField(label, value) {
  if (value === undefined || value === null || value === "") return "";
  return `<div><span>${escapeHTML(label)}</span><strong>${escapeHTML(value)}</strong></div>`;
}

function parseAccount(row) {
  try {
    return JSON.parse(row.dataset.account || "{}");
  } catch {
    return {};
  }
}

function renderLogRow(event) {
  const sev = severityClass(event.severity);
  const eventType = event.event_type || "";
  const message = eventSummary(event);
  const player = actorName(event);
  const license = event.license || "";
  const discord = event.discord || "";
  const steam = event.steam || "";
  const citizenid = event.citizenid || "";
  const server = event.server_name || "";
  const resource = event.resource || "";
  const occurred = event.occurred_at;
  const details = escapeHTML(JSON.stringify(event || {}));
  const playerID = playerIdentifier(event);
  const identity = [license || citizenid, discord, steam].filter(Boolean).map(compactID).join(" · ");
  const chips = metaChips(event).map((chip) => `<span>${escapeHTML(chip)}</span>`).join("");
  const status = reviewStatus(event);
  const archived = Boolean(review(event).archived_at);

  return `
    <article class="log-row ${sev}${archived ? " archived" : ""}" data-log-row data-event="${details}">
      <time>${escapeHTML(formatTime(occurred))}</time>
      <a class="event-type" href="/logs?event_type=${encodeURIComponent(eventType)}">${escapeHTML(eventType)}</a>
      <div class="message">
        <div class="review-line">
          <strong>${escapeHTML(player)}</strong>
          <span class="chip ${reviewStatusClass(status)}">${reviewStatusLabel(status)}</span>
          ${archived ? `<span class="chip warning">${escapeHTML(t("archived"))}</span>` : ""}
        </div>
        <span class="message-summary">${escapeHTML(message)}</span>
        <p>${escapeHTML(identity)}</p>
        ${chips ? `<div class="meta-chips">${chips}</div>` : ""}
      </div>
      <code>${escapeHTML(server)}</code>
      <div class="row-actions">
        ${playerID ? `<a class="chip info" href="/players/${encodeURIComponent(playerID)}">${icon("user", "chip-icon")}<span>${escapeHTML(t("common.player"))}</span></a>` : ""}
        ${event.coords_x !== undefined && event.coords_x !== null ? `<a class="chip success" href="/geo?player=${encodeURIComponent(playerID)}">${icon("crosshair", "chip-icon")}<span>${escapeHTML(t("detail.coords"))}</span></a>` : ""}
        <a class="chip" href="/logs/${encodeURIComponent(event.id || "")}">${icon("link", "chip-icon")}<span>${escapeHTML(t("common.open"))}</span></a>
        <button class="chip ${sev}" type="button" data-open-log>${icon("eye", "chip-icon")}<span>${escapeHTML(resource || sev)}</span></button>
      </div>
    </article>
  `;
}

function startLogStream() {
  const target = document.querySelector("[data-live-log-list]");
  if (!target || !window.EventSource) return;

  state.source = new EventSource("/logs/stream");
  state.source.addEventListener("log", (message) => {
    if (state.paused) return;
    const event = JSON.parse(message.data);
    target.insertAdjacentHTML("afterbegin", renderLogRow(event));
    while (target.children.length > 120) {
      target.lastElementChild.remove();
    }
  });
}

function bindControls() {
  syncReturnTargets();
  bindBulkLogControls();

  document.querySelectorAll("[data-live-toggle]").forEach((button) => {
    button.addEventListener("click", () => {
      state.paused = !state.paused;
      const label = button.querySelector("span");
      if (label) label.textContent = state.paused ? t("dashboard.resume_live") : t("dashboard.pause_live");
      else button.textContent = state.paused ? t("dashboard.resume_live") : t("dashboard.pause_live");
    });
  });

  document.querySelectorAll("[data-clear-log]").forEach((button) => {
    button.addEventListener("click", () => {
      const target = document.querySelector("[data-live-log-list]");
      if (target) target.innerHTML = "";
    });
  });

  document.addEventListener("click", (event) => {
    const accountOpener = event.target.closest("[data-open-account]");
    if (accountOpener) {
      const row = accountOpener.closest("[data-account-row]");
      const dialog = document.querySelector("[data-account-dialog]");
      const detail = document.querySelector("[data-account-detail]");
      const actions = document.querySelector("[data-account-actions]");
      if (dialog && detail && actions && row) {
        const parsed = parseAccount(row);
        const identifier = parsed.identifier || parsed.license || accountValueList(parsed.citizenids) || accountValueList(parsed.discords) || accountValueList(parsed.steams) || accountValueList(parsed.names);
        detail.innerHTML = [
          accountDetailField("License", parsed.license),
          accountDetailField("Names", accountValueList(parsed.names)),
          accountDetailField("Discord", accountValueList(parsed.discords)),
          accountDetailField("Steam", accountValueList(parsed.steams)),
          accountDetailField("CitizenID", accountValueList(parsed.citizenids)),
          accountDetailField("Events", parsed.events),
          accountDetailField("Last Seen", fullTime(parsed.last_seen)),
        ].join("");
        actions.innerHTML = identifier ? [
          `<a class="chip info" href="/players/${encodeURIComponent(identifier)}">${icon("route", "chip-icon")}<span>${escapeHTML(t("player.timeline"))}</span></a>`,
          `<a class="chip" href="/logs?player=${encodeURIComponent(identifier)}">${icon("filter", "chip-icon")}<span>${escapeHTML(t("player.filter_logs"))}</span></a>`,
          parsed.discords && parsed.discords.length ? `<a class="chip" href="/logs?player=${encodeURIComponent(parsed.discords[0])}">Discord ${escapeHTML(t("geo.logs"))}</a>` : "",
        ].join("") : "";
        dialog.showModal();
      }
    }

    const opener = event.target.closest("[data-open-log]");
    if (opener) {
      const row = opener.closest("[data-log-row]");
      const dialog = document.querySelector("[data-log-dialog]");
      const detail = document.querySelector("[data-log-detail]");
      const insight = document.querySelector("[data-log-insight]");
      if (dialog && detail && row) {
        const parsed = parseRowEvent(row);
        const coords = parsed.coords_x !== undefined && parsed.coords_x !== null
          ? [parsed.coords_x, parsed.coords_y, parsed.coords_z].filter((value) => value !== undefined && value !== null).join(", ")
          : "";
        const playerID = playerIdentifier(parsed);
        state.currentLogEvent = parsed;
        state.activeJSONView = "metadata";
        detail.innerHTML = [
          field(t("detail.server"), parsed.server_name),
          field(t("common.event"), parsed.event_type),
          field(t("detail.severity"), parsed.severity),
          field(t("detail.time"), fullTime(parsed.occurred_at)),
          field(t("detail.player"), actorName(parsed)),
          field("License", parsed.license),
          field("Discord", parsed.discord),
          field("Steam", parsed.steam),
          field("CitizenID", parsed.citizenid),
          field("Job", metaValue(parsed, "job")),
          field("Gang", metaValue(parsed, "gang")),
          field(t("detail.item_or_amount"), eventSummary(parsed)),
          field("Resource", parsed.resource),
          field("Source", parsed.player_source),
          field(t("detail.coords"), coords),
          playerID ? `<div class="detail-actions"><a class="chip info" href="/players/${encodeURIComponent(playerID)}">${icon("route", "chip-icon")}<span>${escapeHTML(t("player.timeline"))}</span></a><a class="chip" href="/logs?player=${encodeURIComponent(playerID)}">${icon("filter", "chip-icon")}<span>${escapeHTML(t("player.filter_logs"))}</span></a></div>` : "",
        ].join("");
        if (insight) insight.innerHTML = renderLogInsight(parsed);
        hydrateReviewForm(parsed);
        renderLogJSON(parsed, "metadata");
        hydrateAIJSONImportForm(parsed, "metadata");
        populateAIJSONSelector(parsed);
        dialog.showModal();
      }
    }

    const jsonView = event.target.closest("[data-json-view]");
    if (jsonView && state.currentLogEvent) {
      state.activeJSONView = jsonView.dataset.jsonView || "metadata";
      renderLogJSON(state.currentLogEvent, state.activeJSONView);
      hydrateAIJSONImportForm(state.currentLogEvent, state.activeJSONView);
    }

    if (event.target.closest("[data-load-ai-json-method]")) {
      const card = event.target.closest("[data-ai-json-method-card]");
      const form = document.querySelector("[data-ai-json-form]");
      if (card && form) {
        try {
          const method = JSON.parse(card.dataset.method || "{}");
          const id = form.querySelector("[data-ai-json-id]");
          if (id) id.value = method.id || "";
          form.elements.name.value = method.name || "";
          form.elements.description.value = method.description || "";
          form.elements.source.value = method.source || "metadata";
          form.elements.event_type.value = method.event_type || "";
          form.elements.resource.value = method.resource || "";
          form.elements.prompt.value = method.prompt || "";
          form.elements.spec.value = JSON.stringify(method.spec || {}, null, 2);
          const sample = form.querySelector("[data-ai-json-sample]");
          if (sample && !sample.value.trim()) sample.value = "{}";
          form.scrollIntoView({ behavior: "smooth", block: "start" });
        } catch {
        }
      }
    }

    if (event.target.closest("[data-ai-json-method]")) {
      renderSelectedAIJSONMethod();
    }

    if (event.target.closest("[data-ai-json-format-spec]")) {
      const textarea = document.querySelector("[data-ai-json-spec]");
      if (textarea) {
        try {
          textarea.value = JSON.stringify(JSON.parse(textarea.value || "{}"), null, 2);
        } catch {
          window.alert(t("json.format_error"));
        }
      }
    }

    if (event.target.closest("[data-ai-json-insert-template]")) {
      const textarea = document.querySelector("[data-ai-json-spec]");
      if (textarea) textarea.value = JSON.stringify(fullAIJSONSpecTemplate(), null, 2);
    }

    if (event.target.closest("[data-close-log]")) {
      const dialog = document.querySelector("[data-log-dialog]");
      if (dialog) dialog.close();
    }

    if (event.target.closest("[data-close-account]")) {
      const dialog = document.querySelector("[data-account-dialog]");
      if (dialog) dialog.close();
    }
  });

  document.querySelectorAll("[data-reset-admin-form]").forEach((form) => {
    form.addEventListener("submit", (event) => {
      const select = form.querySelector("[data-admin-reset-select]");
      const id = select ? select.value : "";
      if (!id) {
        event.preventDefault();
        return;
      }
      form.action = `/admins/${encodeURIComponent(id)}/reset-password`;
    });
  });

  document.querySelectorAll("[data-review-form]").forEach((form) => {
    form.addEventListener("submit", (event) => {
      if (event.submitter?.matches("[data-review-archive]") && !window.confirm(t("archive.confirm"))) {
        event.preventDefault();
      }
    });
  });

  document.querySelectorAll("[data-ai-json-form]").forEach((form) => {
    form.addEventListener("submit", (event) => {
      if (event.submitter?.matches("[data-ai-json-update]")) {
        const id = form.querySelector("[data-ai-json-id]")?.value || "";
        if (!id) {
          event.preventDefault();
          window.alert(t("template.select_method_first"));
          return;
        }
        form.action = `/ai-json/methods/${encodeURIComponent(id)}`;
      }

      if (event.submitter?.matches("[data-ai-json-generate]")) {
        const button = event.submitter;
        const label = button.querySelector("[data-ai-json-generate-label]");
        const iconNode = button.querySelector("[data-ai-json-generate-icon]");
        const status = form.querySelector("[data-ai-json-generate-status]");

        button.disabled = true;
        button.setAttribute("aria-busy", "true");
        form.classList.add("ai-json-form-generating");
        if (label) label.textContent = t("ai_json.action.generating");
        if (iconNode) iconNode.setAttribute("hidden", "");
        if (status) status.hidden = false;
      }
    });
  });

  document.querySelectorAll("[data-ai-json-method]").forEach((select) => {
    select.addEventListener("change", renderSelectedAIJSONMethod);
  });

  document.querySelectorAll("[data-ai-provider-select]").forEach((select) => {
    const sync = () => {
      const disabled = select.value === "disabled";
      document.querySelectorAll("[data-ai-provider-input]").forEach((input) => {
        input.disabled = disabled;
      });
    };
    select.addEventListener("change", sync);
    sync();
  });

  document.querySelectorAll("[data-range-minutes]").forEach((button) => {
    button.addEventListener("click", () => {
      const form = document.querySelector("#log-filter-form");
      if (!form) return;
      const minutes = Number(button.dataset.rangeMinutes || "0");
      const until = new Date();
      const since = new Date(until.getTime() - minutes * 60 * 1000);
      const sinceInput = form.querySelector('input[name="since"]');
      const untilInput = form.querySelector('input[name="until"]');
      if (sinceInput) sinceInput.value = datetimeLocal(since);
      if (untilInput) untilInput.value = datetimeLocal(until);
      form.requestSubmit();
    });
  });

  document.querySelectorAll("[data-filter-field]").forEach((button) => {
    const syncPressedState = () => {
      const form = document.querySelector("#log-filter-form");
      if (!form) return;
      const field = button.dataset.filterField || "";
      const input = field ? Array.from(form.elements).find((element) => element.name === field) : null;
      const filterValue = button.dataset.filterValue || "";
      button.setAttribute("aria-pressed", input && input.value === filterValue ? "true" : "false");
    };

    syncPressedState();

    button.addEventListener("click", () => {
      const form = document.querySelector("#log-filter-form");
      if (!form) return;
      const field = button.dataset.filterField || "";
      const input = field ? Array.from(form.elements).find((element) => element.name === field) : null;
      if (!input) return;
      const filterValue = button.dataset.filterValue || "";
      input.value = input.value === filterValue ? "" : filterValue;
      const offset = form.querySelector('input[name="offset"]');
      if (offset) offset.value = "0";
      submitLogFilterForm(form);
    });
  });

  bindGeoTrace();
}

function parseRowEvent(row) {
  try {
    return JSON.parse(row.dataset.event || "{}");
  } catch {
    return {};
  }
}

function submitLogFilterForm(form) {
  const params = new URLSearchParams(new FormData(form));
  [...params.entries()].forEach(([key, value]) => {
    if (!value) params.delete(key);
  });
  const query = params.toString();
  window.location.assign(query ? `${form.action}?${query}` : form.action);
}

function datetimeLocal(date) {
  const parts = timeParts(date);
  return `${parts.year}-${parts.month}-${parts.day}T${parts.hour}:${parts.minute}`;
}

function cssEscape(value) {
  if (window.CSS && typeof window.CSS.escape === "function") return window.CSS.escape(String(value));
  return String(value).replaceAll("\\", "\\\\").replaceAll('"', '\\"').replaceAll("]", "\\]");
}

function parseGeoEvent(node) {
  try {
    return JSON.parse(node.dataset.geoEvent || "{}");
  } catch {
    return {};
  }
}

function setActiveGeo(id) {
  if (!id) return;
  document.querySelectorAll("[data-geo-point], [data-geo-row]").forEach((node) => {
    const active = node.dataset.geoPoint === id || node.dataset.geoRow === id;
    node.classList.toggle("active", active);
  });
}

function severityLabel(value) {
  if (value === "error") return "ERROR";
  if (value === "warning") return "WARN";
  if (value === "success") return "OK";
  return "INFO";
}

function geoEventCoords(event) {
  const parts = [event.coords_x, event.coords_y, event.coords_z]
    .filter((value) => value !== undefined && value !== null && Number.isFinite(Number(value)))
    .map((value) => Number(value).toFixed(1));
  return parts.join(", ");
}

function initGeoMap() {
  const map = document.querySelector("[data-geo-map]");
  if (!map) return null;

  const world = map.querySelector("[data-geo-world]");
  const image = map.querySelector("[data-geo-image]");
  const trace = map.querySelector("[data-geo-trace]");
  const tooltip = map.querySelector("[data-geo-tooltip]");
  const markers = Array.from(map.querySelectorAll("[data-geo-point]"));
  if (!world) return null;

  let cfg = {};
  try {
    cfg = JSON.parse(map.dataset.geoConfig || "{}");
  } catch {
    cfg = {};
  }
  const bounds = {
    minX: Number.isFinite(Number(cfg.min_x)) ? Number(cfg.min_x) : -5610,
    maxX: Number.isFinite(Number(cfg.max_x)) ? Number(cfg.max_x) : 6730,
    minY: Number.isFinite(Number(cfg.min_y)) ? Number(cfg.min_y) : -3850,
    maxY: Number.isFinite(Number(cfg.max_y)) ? Number(cfg.max_y) : 8350,
  };
  if (bounds.minX >= bounds.maxX) {
    bounds.minX = -5610;
    bounds.maxX = 6730;
  }
  if (bounds.minY >= bounds.maxY) {
    bounds.minY = -3850;
    bounds.maxY = 8350;
  }

  if (image && cfg.image_url) {
    const probe = new Image();
    probe.onload = () => {
      image.style.backgroundImage = `url("${String(cfg.image_url).replaceAll('"', '\\"')}")`;
      map.classList.add("has-image");
      map.classList.remove("missing-image");
    };
    probe.onerror = () => map.classList.add("missing-image");
    probe.src = cfg.image_url;
  }

  const events = markers.map((marker) => ({ marker, event: parseGeoEvent(marker) }))
    .filter((item) => Number.isFinite(Number(item.event.coords_x)) && Number.isFinite(Number(item.event.coords_y)));

  const state = { scale: 1, x: 0, y: 0, dragging: false, dragX: 0, dragY: 0, startX: 0, startY: 0 };
  let worldWidth = 0;
  let worldHeight = 0;

  const project = (x, y) => ({
    left: ((Number(x) - bounds.minX) / (bounds.maxX - bounds.minX)) * 100,
    top: (1 - ((Number(y) - bounds.minY) / (bounds.maxY - bounds.minY))) * 100,
  });

  const applyTransform = () => {
    world.style.transform = `translate(${state.x}px, ${state.y}px) scale(${state.scale})`;
  };

  const sizeWorld = () => {
    const rect = map.getBoundingClientRect();
    const ratio = (bounds.maxX - bounds.minX) / (bounds.maxY - bounds.minY);
    let width = rect.width;
    let height = width / ratio;
    if (height < rect.height) {
      height = rect.height;
      width = height * ratio;
    }
    worldWidth = width;
    worldHeight = height;
    world.style.width = `${width}px`;
    world.style.height = `${height}px`;
  };

  const clampPan = () => {
    const rect = map.getBoundingClientRect();
    const scaledW = worldWidth * state.scale;
    const scaledH = worldHeight * state.scale;
    const slackX = Math.max(80, rect.width * 0.25);
    const slackY = Math.max(80, rect.height * 0.25);
    state.x = Math.min(slackX, Math.max(rect.width - scaledW - slackX, state.x));
    state.y = Math.min(slackY, Math.max(rect.height - scaledH - slackY, state.y));
  };

  const zoomAt = (factor, clientX, clientY) => {
    const rect = map.getBoundingClientRect();
    const nextScale = Math.min(6, Math.max(0.85, state.scale * factor));
    const anchorX = clientX - rect.left;
    const anchorY = clientY - rect.top;
    const worldX = (anchorX - state.x) / state.scale;
    const worldY = (anchorY - state.y) / state.scale;
    state.scale = nextScale;
    state.x = anchorX - worldX * state.scale;
    state.y = anchorY - worldY * state.scale;
    clampPan();
    applyTransform();
  };

  const reset = () => {
    state.scale = 1;
    const rect = map.getBoundingClientRect();
    state.x = (rect.width - worldWidth) / 2;
    state.y = (rect.height - worldHeight) / 2;
    applyTransform();
  };

  events.forEach(({ marker, event }) => {
    const point = project(event.coords_x, event.coords_y);
    marker.style.left = `${point.left}%`;
    marker.style.top = `${point.top}%`;
  });

  if (trace && events.length > 1) {
    const groups = {};
    events.forEach(({ event }) => {
      const id = event.player || "unknown";
      if (!groups[id]) groups[id] = [];
      groups[id].push(event);
    });
    let html = "";
    const colorHash = (str) => {
      let hash = 0;
      for (let i = 0; i < str.length; i++) hash = Math.imul(31, hash) + str.charCodeAt(i) | 0;
      return `hsl(${Math.abs(hash) % 360}, 85%, 65%)`;
    };
    Object.entries(groups).forEach(([id, group]) => {
      if (group.length > 1) {
        const points = group.slice().reverse().map((evt) => {
          const point = project(evt.coords_x, evt.coords_y);
          return `${point.left.toFixed(3)},${point.top.toFixed(3)}`;
        }).join(" ");
        html += `<polyline points="${escapeHTML(points)}" stroke="${colorHash(id)}" opacity="0.65"></polyline>`;
      }
    });
    trace.innerHTML = html;
  }

  const showTooltip = (marker) => {
    if (!tooltip) return;
    const event = parseGeoEvent(marker);
    tooltip.className = `geo-tooltip ${severityClass(event.severity)}`;
    tooltip.innerHTML = `
      <strong>${escapeHTML(severityLabel(event.severity))} · ${escapeHTML(event.event_type || "event")}</strong>
      <span>${escapeHTML(event.player || event.message || "")}</span>
      <code>${escapeHTML(geoEventCoords(event))} · ${escapeHTML(fullTime(event.occurred_at))}</code>
    `;
    tooltip.hidden = false;
    const mapRect = map.getBoundingClientRect();
    const markerRect = marker.getBoundingClientRect();
    const left = Math.min(mapRect.width - 18, Math.max(12, markerRect.left - mapRect.left + 14));
    const top = Math.min(mapRect.height - 96, Math.max(12, markerRect.top - mapRect.top - 12));
    tooltip.style.left = `${left}px`;
    tooltip.style.top = `${top}px`;
  };

  const hideTooltip = () => {
    if (tooltip) tooltip.hidden = true;
  };

  const centerMarker = (marker) => {
    const mapRect = map.getBoundingClientRect();
    const markerRect = marker.getBoundingClientRect();
    state.x += mapRect.width / 2 - (markerRect.left - mapRect.left + markerRect.width / 2);
    state.y += mapRect.height / 2 - (markerRect.top - mapRect.top + markerRect.height / 2);
    clampPan();
    applyTransform();
  };

  map.addEventListener("wheel", (event) => {
    event.preventDefault();
    zoomAt(event.deltaY < 0 ? 1.14 : 0.88, event.clientX, event.clientY);
  }, { passive: false });

  map.addEventListener("pointerdown", (event) => {
    if (event.target.closest("[data-geo-point]")) return;
    map.setPointerCapture(event.pointerId);
    map.classList.add("dragging");
    state.dragging = true;
    state.dragX = event.clientX;
    state.dragY = event.clientY;
    state.startX = state.x;
    state.startY = state.y;
  });

  map.addEventListener("pointermove", (event) => {
    if (!state.dragging) return;
    state.x = state.startX + event.clientX - state.dragX;
    state.y = state.startY + event.clientY - state.dragY;
    clampPan();
    applyTransform();
  });

  const stopDragging = () => {
    state.dragging = false;
    map.classList.remove("dragging");
  };
  map.addEventListener("pointerup", stopDragging);
  map.addEventListener("pointercancel", stopDragging);
  map.addEventListener("mouseleave", () => {
    if (!state.dragging) hideTooltip();
  });

  markers.forEach((marker) => {
    marker.addEventListener("mouseenter", () => showTooltip(marker));
    marker.addEventListener("focus", () => showTooltip(marker));
    marker.addEventListener("mouseleave", hideTooltip);
    marker.addEventListener("blur", hideTooltip);
  });

  document.querySelectorAll("[data-geo-zoom]").forEach((button) => {
    button.addEventListener("click", () => {
      const rect = map.getBoundingClientRect();
      zoomAt(button.dataset.geoZoom === "in" ? 1.25 : 0.8, rect.left + rect.width / 2, rect.top + rect.height / 2);
    });
  });
  document.querySelectorAll("[data-geo-reset]").forEach((button) => {
    button.addEventListener("click", reset);
  });

  sizeWorld();
  reset();
  window.addEventListener("resize", () => {
    sizeWorld();
    clampPan();
    applyTransform();
  });
  return { showTooltip, hideTooltip, centerMarker, reset };
}

function bindGeoTrace() {
  const geoMap = initGeoMap();

  document.querySelectorAll("[data-geo-point]").forEach((point) => {
    point.addEventListener("click", () => {
      const id = point.dataset.geoPoint;
      setActiveGeo(id);
      const row = document.querySelector(`[data-geo-row="${cssEscape(id)}"]`);
      if (row) row.scrollIntoView({ block: "nearest", behavior: "smooth" });
    });
    point.addEventListener("focus", () => setActiveGeo(point.dataset.geoPoint));
  });

  document.querySelectorAll("[data-geo-row]").forEach((row) => {
    row.addEventListener("click", (event) => {
      if (event.target.closest("a, button")) return;
      setActiveGeo(row.dataset.geoRow);
      const point = document.querySelector(`[data-geo-point="${cssEscape(row.dataset.geoRow)}"]`);
      if (point) {
        point.focus({ preventScroll: true });
        if (geoMap) {
          geoMap.centerMarker(point);
          geoMap.showTooltip(point);
        }
      }
    });
    row.addEventListener("mouseenter", () => setActiveGeo(row.dataset.geoRow));
  });
}

document.addEventListener("DOMContentLoaded", () => {
  bindControls();
  hydrateTimelineCards();
  startLogStream();
});
