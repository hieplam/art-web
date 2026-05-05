package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Forbidden statuses per spec §5.2.0a: the current API never emits 403,
// non-owner access on private resources collapses to 404. 410/451/511 are
// not emitted by any current handler; introducing them would be a contract
// change. 409 from translated domain errors is forbidden; the only legitimate
// 409 is image fingerprint_mismatch, asserted below.
var forbiddenStatuses = map[int]string{
	403: "non-owner access must collapse to 404 (spec §7.2.1)",
	410: "no current handler emits 410",
	451: "no current handler emits 451",
	511: "no current handler emits 511",
}

type goldenEnvelope struct {
	Body    string              `json:"body"`
	Headers map[string][]string `json:"headers"`
	Status  int                 `json:"status"`
}

func TestForbiddenStatuses_AbsentFromAllGoldens(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("golden", "*.json"))
	if err != nil {
		t.Fatalf("glob goldens: %v", err)
	}
	if len(matches) == 0 {
		t.Skip("no goldens captured yet (PR 0.8 is what populates this)")
	}

	for _, m := range matches {
		bs, err := os.ReadFile(m)
		if err != nil {
			t.Fatalf("read %s: %v", m, err)
		}
		var env goldenEnvelope
		if err := json.Unmarshal(bs, &env); err != nil {
			t.Fatalf("parse %s: %v", m, err)
		}
		if reason, bad := forbiddenStatuses[env.Status]; bad {
			t.Errorf("golden %s emits forbidden status %d: %s", m, env.Status, reason)
		}
		// 409 with a body NOT containing "fingerprint_mismatch" is a translated
		// domain error and must be absent.
		if env.Status == 409 && !contains409Allowed(env.Body) {
			t.Errorf("golden %s emits 409 without fingerprint_mismatch; "+
				"likely a translated domain error (spec §7.2.1)", m)
		}
	}
}

func contains409Allowed(body string) bool {
	const expected = `"error":"fingerprint_mismatch"`
	for i := 0; i+len(expected) <= len(body); i++ {
		if body[i:i+len(expected)] == expected {
			return true
		}
	}
	return false
}
