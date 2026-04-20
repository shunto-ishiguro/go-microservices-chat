package auth_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"

	"go-microservices-chat/pkg/auth"
)

func TestRequesterID(t *testing.T) {
	tests := []struct {
		name   string
		md     metadata.MD
		wantID string
		wantOK bool
	}{
		{
			name:   "metadata missing",
			md:     nil,
			wantOK: false,
		},
		{
			name:   "header missing",
			md:     metadata.Pairs("other", "val"),
			wantOK: false,
		},
		{
			name:   "empty value",
			md:     metadata.Pairs(auth.MetadataKeyUserID, ""),
			wantOK: false,
		},
		{
			name:   "present",
			md:     metadata.Pairs(auth.MetadataKeyUserID, "alice-uuid"),
			wantID: "alice-uuid",
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.md != nil {
				ctx = metadata.NewIncomingContext(ctx, tt.md)
			}
			got, ok := auth.RequesterID(ctx)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.wantID {
				t.Errorf("id = %q, want %q", got, tt.wantID)
			}
		})
	}
}

func TestPropagateRequester(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs(auth.MetadataKeyUserID, "alice-uuid"))
	out := auth.PropagateRequester(ctx)

	md, ok := metadata.FromOutgoingContext(out)
	if !ok {
		t.Fatal("no outgoing metadata")
	}
	if got := md.Get(auth.MetadataKeyUserID); len(got) != 1 || got[0] != "alice-uuid" {
		t.Errorf("outgoing %s = %v", auth.MetadataKeyUserID, got)
	}
}
