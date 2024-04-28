package util

import "testing"

func TestAuthInfo_EncodeToString(t *testing.T) {
	type fields struct {
		Username string
		Password string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "test1",
			fields: fields{
				Username: "test",
				Password: "test",
			},
			want: "eyJ1c2VybmFtZSI6InRlc3QiLCJwYXNzd29yZCI6InRlc3QifQ==",
		},
		{
			name: "empty password",
			fields: fields{
				Username: "test",
				Password: "",
			},
			want: "eyJ1c2VybmFtZSI6InRlc3QifQ==",
		},
		{
			name: "empty",
			fields: fields{
				Username: "",
				Password: "",
			},
			want: "e30=",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &AuthInfo{
				Username: tt.fields.Username,
				Password: tt.fields.Password,
			}
			if got := i.EncodeToString(); got != tt.want {
				t.Errorf("EncodeToString() = %v, want %v", got, tt.want)
			}
		})
	}
}
