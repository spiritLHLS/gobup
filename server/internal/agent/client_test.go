package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFileCheckRequestMarshalsExplicitZeroMinSize(t *testing.T) {
	zero := int64(0)
	body, err := json.Marshal(FileCheckRequest{MinSize: &zero})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"minSize":0`) {
		t.Fatalf("expected explicit zero minSize to be marshaled, got %s", body)
	}
}
