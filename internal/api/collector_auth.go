package api

import (
	"fmt"
	"net/http"
	"strings"
)

// AdminClaim names the claim an OIDC token must carry to manage collectors:
// the value at the dotted path Claim, such as "realm_access.roles", must
// equal Value or, for an array, contain it.
type AdminClaim struct {
	Claim string
	Value string
}

// requireAdmin wraps next so that only principals carrying the claim reach
// it. With no claim configured nobody does, and API tokens never do: they
// carry no claims.
func requireAdmin(claim AdminClaim, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := principalFrom(r.Context())

		switch {
		case claim.Claim == "" || claim.Value == "":
			writeError(w, http.StatusForbidden,
				"managing collectors is disabled: set COLLECTOR_ADMIN_CLAIM and COLLECTOR_ADMIN_VALUE")
		case p.APITokenID != nil:
			writeError(w, http.StatusForbidden, "API tokens cannot manage collectors")
		case !claimContains(p.Claims, claim.Claim, claim.Value):
			writeError(w, http.StatusForbidden,
				fmt.Sprintf("managing collectors requires the claim %s to contain %q", claim.Claim, claim.Value))
		default:
			next.ServeHTTP(w, r)
		}
	})
}

// claimContains reports whether the claim at the dotted path is the string
// want or an array holding it.
func claimContains(claims map[string]any, path, want string) bool {
	var v any = claims

	for key := range strings.SplitSeq(path, ".") {
		obj, ok := v.(map[string]any)
		if !ok {
			return false
		}

		v = obj[key]
	}

	switch v := v.(type) {
	case string:
		return v == want
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == want {
				return true
			}
		}
	}

	return false
}
