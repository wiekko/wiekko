package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"nexmap.net/internal/config"
	"nexmap.net/internal/store"
)

// testProductID is the product unit tests act in; testOwnership reports every
// entity as belonging to it, so tests that are not about ownership need not
// model it. TestOwnership* below cover the checks themselves.
const testProductID = "product-001"

func testOwnership() *ownership {
	return &ownership{productOf: func(context.Context, string, string) (string, error) { return testProductID, nil }}
}

// ownedBy reports each listed ID as belonging to the given product and
// everything else as missing.
func ownedBy(products map[string]string) *ownership {
	return &ownership{productOf: func(_ context.Context, _ string, id string) (string, error) {
		if p, ok := products[id]; ok {
			return p, nil
		}
		return "", errors.New("not found")
	}}
}

func TestOwnershipRequire(t *testing.T) {
	ctx := context.Background()
	o := ownedBy(map[string]string{"req-a": "product-a", "req-b": "product-b"})

	if err := o.require(ctx, "product-a", ref{kindRequirement, "req-a"}, ref{kindMilestone, ""}); err != nil {
		t.Fatalf("own entity plus empty optional ref: %v", err)
	}
	for _, tc := range []struct {
		name string
		r    ref
	}{
		{"other product", ref{kindRequirement, "req-b"}},
		{"missing", ref{kindRequirement, "req-x"}},
	} {
		err := o.require(ctx, "product-a", tc.r)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: err = %v, want ErrNotFound", tc.name, err)
		}
		if errors.Is(err, ErrForbidden) {
			t.Errorf("%s: must not reveal the entity exists", tc.name)
		}
	}
	// A nil checker fails closed.
	var unset *ownership
	if err := unset.require(ctx, "product-a", ref{kindRequirement, "req-a"}); !errors.Is(err, errOwnershipUnset) {
		t.Errorf("nil checker: err = %v", err)
	}
}

// TestNewWiresOwnership guards against a service gaining an owns field that
// New forgets to set; such a service would fail every request (fail closed),
// but this reports it at test time instead.
func TestNewWiresOwnership(t *testing.T) {
	svcs := New(&store.Store{}, nil, &config.Config{}, nil)
	v := reflect.ValueOf(svcs).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() != reflect.Pointer || f.IsNil() {
			continue
		}
		owns := f.Elem().FieldByName("owns")
		if owns.IsValid() && owns.IsNil() {
			t.Errorf("%s.owns is not wired in New", v.Type().Field(i).Name)
		}
	}
}
