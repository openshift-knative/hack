package rhel

import "testing"

func Test_extractRHELVersion(t *testing.T) {
	tests := []struct {
		str     string
		want    string
		wantErr bool
	}{
		{
			str:  "golang-builder:rhel_8_golang_1.22",
			want: "8",
		}, {
			str:  "golang-builder:rhel-18_golang_1.22",
			want: "18",
		}, {
			str:  "foobar.com/ubi8/ubi-minimal",
			want: "8",
		}, {
			str:  "foobar.com/ubi18/ubi-minimal",
			want: "18",
		}, {
			str:     "nothing to match",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			got, err := extractRHELVersion(tt.str)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractRHELVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractRHELVersion() got = %v, want %v", got, tt.want)
			}
		})
	}
}
