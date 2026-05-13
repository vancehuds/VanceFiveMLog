package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	endpoint := flag.String("endpoint", "http://127.0.0.1:8080/api/v1/events", "events API endpoint")
	key := flag.String("key", "", "server API key")
	flag.Parse()

	if *key == "" {
		*key = os.Getenv("VFL_API_KEY")
	}
	if *key == "" {
		fmt.Fprintln(os.Stderr, "set -key or VFL_API_KEY")
		os.Exit(2)
	}

	now := time.Now().UTC()
	payload := map[string]any{
		"events": []map[string]any{
			{
				"event_type":  "player_connecting",
				"severity":    "info",
				"source":      12,
				"player_name": "Vance",
				"license":     "license:110000112233445566778899",
				"discord":     "discord:123456789012345678",
				"citizenid":   "QBX78231",
				"resource":    "qbx_core",
				"message":     "Vance joined the server",
				"coords":      map[string]float64{"x": 219.2, "y": -810.1, "z": 30.7},
				"metadata":    map[string]any{"routing_bucket": 0, "ping": 42, "character_name": "Wei Chen", "job": "police:sergeant", "gang": "none"},
				"occurred_at": now.Add(-4 * time.Minute),
			},
			{
				"event_type":  "money_change",
				"severity":    "warning",
				"source":      12,
				"player_name": "Vance",
				"license":     "license:110000112233445566778899",
				"citizenid":   "QBX78231",
				"resource":    "qbx_core",
				"message":     "cash remove 250: vehicle purchase",
				"coords":      map[string]float64{"x": 116.8, "y": -1949.6, "z": 20.7},
				"metadata":    map[string]any{"character_name": "Wei Chen", "job": "police:sergeant", "money_type": "cash", "amount": 250, "operation": "remove", "reason": "vehicle purchase", "balance": 1420},
				"occurred_at": now.Add(-2 * time.Minute),
			},
			{
				"event_type":  "inventory_diff",
				"severity":    "warning",
				"source":      12,
				"player_name": "Vance",
				"license":     "license:110000112233445566778899",
				"citizenid":   "QBX78231",
				"resource":    "ox_inventory",
				"message":     "inventory changed: Marked Bills -5, Lockpick +2",
				"coords":      map[string]float64{"x": 116.8, "y": -1949.6, "z": 20.7},
				"metadata": map[string]any{
					"character_name": "Wei Chen",
					"job":            "police:sergeant",
					"category":       "inventory",
					"change_count":   2,
					"context_text":   "event=swapItems from=player to=stash",
					"changes": []map[string]any{
						{"name": "markedbills", "label": "Marked Bills", "before": 12, "after": 7, "delta": -5},
						{"name": "lockpick", "label": "Lockpick", "before": 1, "after": 3, "delta": 2},
					},
				},
				"occurred_at": now.Add(-1 * time.Minute),
			},
			{
				"event_type":  "player_killed",
				"severity":    "error",
				"source":      31,
				"player_name": "Ghost",
				"license":     "license:220000998877665544332211",
				"citizenid":   "QBX11382",
				"resource":    "baseevents",
				"message":     "Ghost killed by Vance",
				"coords":      map[string]float64{"x": -43.1, "y": -1098.2, "z": 26.4},
				"metadata":    map[string]any{"weapon": "WEAPON_PISTOL", "killer": 12},
				"occurred_at": now,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	req, err := http.NewRequest(http.MethodPost, *endpoint, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+*key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "seed failed: %s\n", resp.Status)
		os.Exit(1)
	}
	fmt.Println("seed events inserted")
}
