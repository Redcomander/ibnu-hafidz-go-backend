package handlers

import (
	"testing"

	"github.com/ibnu-hafidz/web-v2/internal/models"
)

func TestUserCanManageKamar(t *testing.T) {
	kamar := &models.Kamar{
		ID:          10,
		WaliKamarID: 2,
		SecondaryWalies: []*models.User{
			{ID: 3},
		},
	}

	t.Run("owner can manage room", func(t *testing.T) {
		user := &models.User{ID: 2}
		if !userCanManageKamar(user, kamar) {
			t.Fatal("expected main wali to manage room")
		}
	})

	t.Run("secondary wali can manage room", func(t *testing.T) {
		user := &models.User{ID: 3}
		if !userCanManageKamar(user, kamar) {
			t.Fatal("expected secondary wali to manage room")
		}
	})

	t.Run("super admin can manage any room", func(t *testing.T) {
		user := &models.User{ID: 99, Roles: []models.Role{{Name: "super_admin"}}}
		if !userCanManageKamar(user, kamar) {
			t.Fatal("expected super admin to manage room")
		}
	})

	t.Run("unassigned user cannot manage room", func(t *testing.T) {
		user := &models.User{ID: 42}
		if userCanManageKamar(user, kamar) {
			t.Fatal("expected unassigned user to be denied")
		}
	})

	t.Run("room owner is not allowed to delete without admin role", func(t *testing.T) {
		user := &models.User{ID: 2}
		if isKamarAdminUser(user) {
			t.Fatal("owner should not count as admin")
		}
	})
}
