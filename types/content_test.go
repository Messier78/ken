package types

import (
	"context"
	"testing"
)

func Test_ChangeValueInCtx(t *testing.T) {
	ctx := context.Background()
	cont := &Content{
		Domain: "main",
		App:    "Ultraman",
	}
	cctx := NewContext(ctx, cont)
	serviceHandler(cctx)
	if cont.Domain != "service" {
		t.Error("!!!!!!!!!!!!!!!!!!!!!!!")
	}
}

func serviceHandler(ctx context.Context) {
	if cont, ok := FromContext(ctx); ok {
		cont.Domain = "service"
	}
}
