package record

import "testing"

func TestValidateNodeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "valid simple", value: "alpha"},
		{name: "valid hyphen", value: "alpha-01"},
		{name: "empty rejected", value: "", wantErr: true},
		{name: "uppercase rejected", value: "Alpha", wantErr: true},
		{name: "too long rejected", value: "abcdefghijklmnop", wantErr: true},
		{name: "leading hyphen rejected", value: "-alpha", wantErr: true},
		{name: "trailing hyphen rejected", value: "alpha-", wantErr: true},
		{name: "underscore rejected", value: "alpha_beta", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateNodeName(tc.value)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}
