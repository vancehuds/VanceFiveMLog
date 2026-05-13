package auth

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "correct horse battery staple") {
		t.Fatal("expected password to match")
	}
	if CheckPassword(hash, "wrong") {
		t.Fatal("expected wrong password to fail")
	}
}

func TestNormalizeRole(t *testing.T) {
	tests := []struct {
		name string
		role string
		want string
		ok   bool
	}{
		{name: "default", role: "", want: RoleAdmin, ok: true},
		{name: "trim", role: " viewer ", want: RoleViewer, ok: true},
		{name: "owner", role: RoleOwner, want: RoleOwner, ok: true},
		{name: "invalid", role: "operator", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeRole(tt.role)
			if ok != tt.ok {
				t.Fatalf("ok = %v", ok)
			}
			if ok && got != tt.want {
				t.Fatalf("role = %q", got)
			}
		})
	}
}

func TestAdminPermissions(t *testing.T) {
	owner := Admin{Role: RoleOwner}
	if !owner.CanManageSettings() || !owner.CanManageAdmins() || !owner.CanManageServers() || !owner.CanManageLogs() {
		t.Fatal("expected owner to manage everything")
	}

	admin := Admin{Role: RoleAdmin}
	if admin.CanManageSettings() || admin.CanManageAdmins() || !admin.CanManageServers() || !admin.CanManageLogs() {
		t.Fatal("expected admin to manage servers and logs")
	}

	viewer := Admin{Role: RoleViewer}
	if viewer.CanManageSettings() || viewer.CanManageAdmins() || viewer.CanManageServers() || viewer.CanManageLogs() {
		t.Fatal("expected viewer to be read-only")
	}

	unknown := Admin{Role: "legacy"}
	if unknown.CanManageSettings() || unknown.CanManageAdmins() || unknown.CanManageServers() || unknown.CanManageLogs() {
		t.Fatal("expected unknown role to have no write permissions")
	}
}
