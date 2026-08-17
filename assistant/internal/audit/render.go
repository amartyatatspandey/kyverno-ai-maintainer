package audit

import (
	"fmt"
	"strings"
)

// Render produces the maintainer-facing "audit show" view (AUDIT.md sample).
func Render(events []Event) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }

	for _, ev := range events {
		switch ev.Type {
		case "run_started":
			w("Run %s — %v — %v", ev.RunID, ev.Data["workflow"], ev.Data["entity"])
			w("Trigger: %v  |  Model: %v  |  Config: %v", ev.Data["trigger"], ev.Data["model"], ev.Data["config_path"])
			w("")
			w("What happened:")
		case "tool_called":
			w("  → tool %v(%v)", ev.Data["tool"], compact(ev.Data["args"]))
		case "classification":
			w("  • classified: %v", compact(ev.Data))
		case "llm_call":
			w("  • LLM [%v] (%v tokens)  [advisory]: %v", ev.Data["insertion_point"], ev.Data["tokens"], truncate(fmt.Sprint(ev.Data["summary"]), 160))
		case "test_selection":
			w("  • test selection: %v", compact(ev.Data))
		case "test_results":
			w("  • sandbox results: %v", compact(ev.Data))
		case "policy_decision":
			verdict := "DENY"
			if ev.Data["allowed"] == true {
				verdict = "ALLOW"
			}
			w("  POLICY %v → %s  [bound to %v]", ev.Data["action"], verdict, ev.Data["bound_sha"])
			if rules, ok := ev.Data["rules"].([]any); ok {
				for _, r := range rules {
					rm, _ := r.(map[string]any)
					mark := "✓"
					if rm["pass"] != true {
						mark = "✗"
					}
					w("      %s %v — %v", mark, rm["rule"], rm["reason"])
				}
			}
		case "action_executed":
			w("  ⚡ EXECUTED %v → %v", ev.Data["action"], compact(ev.Data["result"]))
		case "action_skipped":
			w("  ⏸ SKIPPED %v (%v)", ev.Data["action"], ev.Data["reason"])
		case "kill_switch_checked":
			if ev.Data["state"] == true {
				w("  ⛔ kill switch ACTIVE (%v)", ev.Data["source"])
			}
		case "run_finished":
			w("")
			w("Outcome: %v", ev.Data["outcome"])
			if u, ok := ev.Data["undo"]; ok {
				w("To undo: %v", u)
			}
			w("To stop the bot: add label 'ai-hold' or set repo variable AI_MAINTAINER_PAUSED=true")
		}
	}
	return b.String()
}

func compact(v any) string { return truncate(fmt.Sprint(v), 220) }

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
