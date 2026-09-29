package app

import "testing"

func TestValidateMySQLServerVersion(t *testing.T) {
	cases := []struct {
		name    string
		version string
		wantErr bool
	}{
		{name: "minimum supported", version: "8.0.16", wantErr: false},
		{name: "newer supported", version: "8.4.5", wantErr: false},
		{name: "older mysql", version: "8.0.15", wantErr: true},
		{name: "mariadb", version: "10.11.8-MariaDB", wantErr: true},
		{name: "unknown", version: "not-a-version", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateMySQLServerVersion(tc.version); (err != nil) != tc.wantErr {
				t.Fatalf("validateMySQLServerVersion(%q) error = %v, wantErr=%v", tc.version, err, tc.wantErr)
			}
		})
	}
}
