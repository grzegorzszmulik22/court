package daemon

// Host-only storage (spec/01 §11.3): a URL input keeps its host name and
// nothing else — scheme, userinfo, port, path, query and fragment never
// reach the stored event, wherever a URL can hide a token.

import (
	"encoding/json"
	"strings"
	"testing"
)

// urlCases are URLs whose non-host parts are marked canaries; want is the
// only string the ledger may keep.
var urlCases = []struct {
	name   string
	url    string
	want   string
	forbid []string
}{
	{"supervisor", "https://user:pw@api.example.com:8443/v1/x?token=SECRET#frag",
		"api.example.com",
		[]string{"https", "user", "pw", "8443", "v1/x", "token", "SECRET", "frag", "@"}},
	{"userinfo-multi-at", "https://uuSec9r:pp@x@safe-host.example/",
		"safe-host.example", []string{"uuSec9r", "pp", "x@"}},
	{"user-no-pass", "https://uuSec9r@safe-host.example/ppSec9x",
		"safe-host.example", []string{"uuSec9r", "ppSec9x"}},
	{"port-path-query", "safe-host.example:8437/ppSec9x?qqSec9y=ttSec9z",
		"safe-host.example", []string{"8437", "ppSec9x", "qqSec9y", "ttSec9z"}},
	{"scheme-relative", "//safe-host.example/ppSec9x",
		"safe-host.example", []string{"ppSec9x"}},
	{"bare", "safe-host.example", "safe-host.example", nil},
	{"upper-scheme", "HTTPS://safe-host.example:443/",
		"safe-host.example", []string{"HTTPS", ":443"}},
	{"ipv6", "https://[2001:db8::1]:8443/ppSec9x",
		"2001:db8::1", []string{"8443", "ppSec9x"}},
	{"trailing-at-in-path", "https://safe-host.example/pp@x",
		"safe-host.example", []string{"pp@x", "pp"}},
	{"frag-only", "safe-host.example#ffSec9w",
		"safe-host.example", []string{"ffSec9w"}},
}

func TestHostOf(t *testing.T) {
	for _, tc := range urlCases {
		if got := hostOf(tc.url); got != tc.want {
			t.Errorf("%s: hostOf(%q) = %q, want %q", tc.name, tc.url, got, tc.want)
		}
	}
}

// TestBuildToolEvent_HostsOnly runs every tool input the mapping table
// routes to host names through buildToolEvent: the stored hosts equal the
// URL's host and no other URL part appears anywhere in the marshalled
// body — a token riding in userinfo, port, path, query or fragment is
// gone, not merely out of the hosts list.
func TestBuildToolEvent_HostsOnly(t *testing.T) {
	mapped := 0
	for tool, keys := range inputKeys {
		for _, field := range keys.hosts {
			for _, tc := range urlCases {
				mapped++
				te, err := buildToolEvent(&hookPayload{
					ToolName:  tool,
					ToolUseID: "toolu_h",
					ToolInput: map[string]any{field: tc.url},
				}, "ok", "")
				if err != nil {
					t.Fatalf("%s %s %s: %v", tool, field, tc.name, err)
				}
				if len(te.Fields.Hosts) != 1 || te.Fields.Hosts[0] != tc.want {
					t.Fatalf("%s.%s %s: hosts = %v, want [%q]",
						tool, field, tc.name, te.Fields.Hosts, tc.want)
				}
				raw, err := json.Marshal(te)
				if err != nil {
					t.Fatal(err)
				}
				for _, bad := range tc.forbid {
					if strings.Contains(string(raw), bad) {
						t.Fatalf("%s.%s %s: body stores %q: %s",
							tool, field, tc.name, bad, raw)
					}
				}
			}
		}
	}
	if mapped == 0 {
		t.Fatal("no tool input is mapped to hosts — the check would test nothing")
	}
}
