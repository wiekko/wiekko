package controller

import (
	"context"
	"net/http"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
)

// refResolver turns an entity reference from the URL into the entity's ID.
type refResolver func(ctx context.Context, user *model.User, productID, ref string) (string, error)

// ResolveRefs rewrites the named path values from human-readable identifiers
// ("FR-12", "AC-42") to entity IDs before the handler runs, so every route
// below /requirements/{id} or /constraints/{id} accepts either form without
// each handler knowing about identifiers. IDs pass through unchanged. It must
// run after authentication.
func ResolveRefs(resolve refResolver, names ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := middleware.UserFromContext(r.Context())
			productID := r.PathValue("productId")
			for _, name := range names {
				ref := r.PathValue(name)
				if ref == "" {
					continue
				}
				id, err := resolve(r.Context(), user, productID, ref)
				if err != nil {
					writeServiceError(w, err)
					return
				}
				r.SetPathValue(name, id)
			}
			next.ServeHTTP(w, r)
		})
	}
}
