package api

import (
	"fmt"
	"net/http"
)

// groupsClaim is the OIDC claim listing the groups a user is in, as Authentik
// and Keycloak's group mapper send it.
const groupsClaim = "groups"

// requireAdminGroup wraps next so that only members of group reach it. With
// no group configured nobody does, and API tokens never do: they carry no
// claims.
func requireAdminGroup(group string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := principalFrom(r.Context())

		switch {
		case group == "":
			writeError(w, http.StatusForbidden, "managing collectors is disabled: set COLLECTOR_ADMIN_GROUP")
		case p.APITokenID != nil:
			writeError(w, http.StatusForbidden, "API tokens cannot manage collectors")
		case !inGroup(p.Claims, group):
			writeError(w, http.StatusForbidden, fmt.Sprintf("managing collectors requires membership in the group %q", group))
		default:
			next.ServeHTTP(w, r)
		}
	})
}

// inGroup reports whether the groups claim lists group. A single group sent
// as a plain string counts too.
func inGroup(claims map[string]any, group string) bool {
	switch groups := claims[groupsClaim].(type) {
	case string:
		return groups == group
	case []any:
		for _, g := range groups {
			if s, ok := g.(string); ok && s == group {
				return true
			}
		}
	}

	return false
}
