package internal

import (
	"context"
	"testing"

	formatsv1 "github.com/Muxcore-Media/media-custom-formats/proto/formatsv1"
)

func TestReleaseProfilesRPC(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()

	created, err := m.UpsertReleaseProfile(ctx, &formatsv1.UpsertReleaseProfileRequest{
		Name: "Preferred Groups",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created.GetProfile().GetEnabled() {
		t.Fatal("expected enabled=true by default on create")
	}
	if created.GetProfile().GetId() == "" {
		t.Fatal("expected generated id")
	}

	list, err := m.ListReleaseProfiles(ctx, &formatsv1.ListReleaseProfilesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.GetProfiles()) != 1 {
		t.Fatalf("profiles=%d want 1", len(list.GetProfiles()))
	}

	disabled := false
	updated, err := m.UpsertReleaseProfile(ctx, &formatsv1.UpsertReleaseProfileRequest{
		Id:      created.GetProfile().GetId(),
		Name:    "Preferred Groups",
		Enabled: &disabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.GetProfile().GetEnabled() {
		t.Fatal("expected enabled=false after update")
	}

	if _, err := m.DeleteReleaseProfile(ctx, &formatsv1.DeleteReleaseProfileRequest{Id: created.GetProfile().GetId()}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.DeleteReleaseProfile(ctx, &formatsv1.DeleteReleaseProfileRequest{Id: "missing"}); err == nil {
		t.Fatal("expected not-found on second delete")
	}
}

func TestDeleteFormatProfileNotFound(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()
	if _, err := m.DeleteFormat(ctx, &formatsv1.DeleteFormatRequest{Id: "missing"}); err == nil {
		t.Fatal("expected format not found")
	}
	if _, err := m.DeleteProfile(ctx, &formatsv1.DeleteProfileRequest{Id: "missing"}); err == nil {
		t.Fatal("expected profile not found")
	}
}
