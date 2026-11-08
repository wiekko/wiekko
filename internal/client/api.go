package client

import (
	"context"
	"net/http"
	"net/url"
)

// ServerVersion is the response of GET /version.
type ServerVersion struct {
	Version  string `json:"version"`
	APILevel int    `json:"api_level"`
}

// User is the authenticated user as returned by GET /user/me.
type User struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	IsAdmin     bool   `json:"is_admin"`
	Status      string `json:"status"`
}

// Product is a product as returned by the product endpoints.
type Product struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	EstimationMode string `json:"estimation_mode"`
	WorkflowMode   string `json:"workflow_mode"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// Version returns the server's version and API level. It needs no token.
func (c *Client) Version(ctx context.Context) (*ServerVersion, error) {
	var v ServerVersion
	if err := c.Do(ctx, http.MethodGet, "/version", nil, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// Me returns the user the token belongs to.
func (c *Client) Me(ctx context.Context) (*User, error) {
	var u User
	if err := c.Do(ctx, http.MethodGet, "/user/me", nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// Products lists the products the user can see.
func (c *Client) Products(ctx context.Context) ([]Product, error) {
	var ps []Product
	if err := c.Do(ctx, http.MethodGet, "/products", nil, &ps); err != nil {
		return nil, err
	}
	return ps, nil
}

// Product returns one product by ID.
func (c *Client) Product(ctx context.Context, id string) (*Product, error) {
	var p Product
	if err := c.Do(ctx, http.MethodGet, "/products/"+url.PathEscape(id), nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
