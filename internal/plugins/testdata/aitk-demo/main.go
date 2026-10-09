// aitk-demo is a reference plugin used by tests: it implements every hook.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "manifest" {
		fmt.Println(`{"schema":"aitk.plugin.manifest/v1","name":"demo","version":"1.0.0","hooks":["brief.sections","close.after","doctor.checks","knowledge.search"]}`)
		return
	}
	if len(os.Args) < 3 || os.Args[1] != "hook" {
		os.Exit(2)
	}
	var req map[string]any
	json.NewDecoder(os.Stdin).Decode(&req)
	var data any
	switch os.Args[2] {
	case "brief.sections":
		if os.Getenv("AITK_DEMO_SLOW") != "" {
			time.Sleep(3 * time.Second)
		}
		settings, _ := req["payload"].(map[string]any)["settings"].(map[string]any)
		data = []map[string]any{{"title": "Demo memory", "body": fmt.Sprintf("namespace=%v", settings["namespace"]), "rank": 1}}
	case "close.after":
		data = nil
	case "doctor.checks":
		data = []map[string]any{{"name": "demo backend", "status": "warn", "detail": "demo is a test plugin"}}
	case "knowledge.search":
		data = []map[string]any{{"text": "Demo plugin remembers: deploy on Fridays is forbidden", "score": 9, "source": "demo"}}
	}
	out, _ := json.Marshal(map[string]any{"schema": "aitk.plugin.response/v1", "ok": true, "data": data})
	os.Stdout.Write(out)
}
