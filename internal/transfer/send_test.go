package transfer

import "testing"

func TestValidateIPv6Address(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		address string
		wantErr bool
	}{
		{
			name:    "valid ipv6",
			address: "[2001:db8::1]:4242",
		},
		{
			name:    "ipv4 rejected",
			address: "127.0.0.1:4242",
			wantErr: true,
		},
		{
			name:    "missing brackets rejected",
			address: "2001:db8::1:4242",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateIPv6Address(tc.address)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}
