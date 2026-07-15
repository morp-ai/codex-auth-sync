package hub

import (
	"net/http/httptest"
	"testing"
)

func TestParseIntQueryInt(t *testing.T) {
	tests := []struct {
		query   string
		want    int
		wantErr bool
	}{
		{query: "", want: 500},
		{query: "1", want: 1},
		{query: "500", want: 500},
		{query: "0", wantErr: true},
		{query: "501", wantErr: true},
		{query: "9223372036854775808", wantErr: true},
		{query: "not-a-number", wantErr: true},
	}
	for _, test := range tests {
		request := httptest.NewRequest("GET", "/v1/events?limit="+test.query, nil)
		got, err := parseIntQueryInt(request, "limit", 500, 1, 500)
		if (err != nil) != test.wantErr {
			t.Fatalf("query %q: unexpected error state: %v", test.query, err)
		}
		if !test.wantErr && got != test.want {
			t.Fatalf("query %q: got %d, want %d", test.query, got, test.want)
		}
	}
}
