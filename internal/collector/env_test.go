package collector

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nils-witt/address-serve/internal/collector/contract"
)

func TestBaseEnv_PassesOnlyTheAllowlist(t *testing.T) {
	t.Parallel()

	appEnv := map[string]string{
		"PATH": "/bin", "HTTPS_PROXY": "http://proxy", "DATABASE_URL": "postgres://secret",
		"POSTGRES_PASSWORD": "secret", "OIDC_ISSUER_URL": "https://sso",
	}
	lookup := func(k string) (string, bool) { v, ok := appEnv[k]; return v, ok }

	got := baseEnv(lookup, "bonn")
	want := []string{"PATH=/bin", "HTTPS_PROXY=http://proxy", "ADDRESS_SERV_CONTRACT_VERSION=1", "ADDRESS_SERV_COLLECTOR_ID=bonn"}

	if !slices.Equal(got, want) {
		t.Errorf("baseEnv = %v, want %v", got, want)
	}
}

func TestCollectEnv(t *testing.T) {
	t.Parallel()

	schema := []contract.SettingSpec{
		{Key: "url", Type: contract.SettingString, Default: json.RawMessage(`"https://default"`)},
		{Key: "api_key", Type: contract.SettingSecret},
		{Key: "limit", Type: contract.SettingInteger},
		{Key: "dry_run", Type: contract.SettingBoolean, Default: json.RawMessage(`false`)},
	}
	runID := uuid.New()

	env, err := collectEnv([]string{"PATH=/bin"}, runID, schema, map[string]json.RawMessage{
		"api_key": json.RawMessage(`"s3cret"`), "dry_run": json.RawMessage(`true`),
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"PATH=/bin", "ADDRESS_SERV_RUN_ID=" + runID.String(),
		"COLLECTOR_SETTING_URL=https://default", "COLLECTOR_SETTING_API_KEY=s3cret", "COLLECTOR_SETTING_DRY_RUN=true",
	}
	if !slices.Equal(env, want) {
		t.Errorf("collectEnv = %v, want %v", env, want)
	}

	_, err = collectEnv(nil, runID, schema, map[string]json.RawMessage{"limit": json.RawMessage(`"ten"`)})
	if err == nil || !strings.Contains(err.Error(), "setting limit") {
		t.Errorf("invalid stored value: err = %v", err)
	}
}

func TestTailBuffer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		writes []string
		want   string
	}{
		{name: "fits", writes: []string{"ab", "cd"}, want: "abcd"},
		{name: "keeps the end", writes: []string{"abcdef", "gh"}, want: "[earlier output truncated]\ndefgh"},
		{name: "drops NUL bytes", writes: []string{"a\x00b"}, want: "ab"},
		{name: "replaces invalid UTF-8", writes: []string{"a\xffb"}, want: "a�b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := &tailBuffer{max: 5}
			for _, w := range tt.writes {
				if n, err := b.Write([]byte(w)); n != len(w) || err != nil {
					t.Fatalf("Write = %d, %v", n, err)
				}
			}

			if got := b.String(); got != tt.want {
				t.Errorf("String = %q, want %q", got, tt.want)
			}
		})
	}
}
