package selector

import (
	"context"
	"strings"
	"testing"
	"time"

	"optionsbacktest/models"
)

func TestDeltaSelectorNotImplemented(t *testing.T) {
	contract, err := NewDeltaSelector(models.Call, 0.2).Select(context.Background(), 7, nil, time.Time{}, nil)
	if contract != nil || err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("expected not implemented, got contract=%v error=%v", contract, err)
	}
}
