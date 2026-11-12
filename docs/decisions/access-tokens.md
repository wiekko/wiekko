# Access token decisions

Decisions behind restricted personal access tokens (SC-401 – SC-409): what was chosen, and why. What each
scope covers is described in the [REST API reference](../api/rest.md#restricted-tokens).

## Three independent limits

A token can be limited to products, capped at a role, and restricted to scopes. They answer different
questions: *where* (products), *as whom* (role cap), *on what* (scopes). The role ladder alone cannot say
"defects, read-only", and scopes alone cannot keep a product owner's token from activating items, so
both exist. All three are optional; a token without restrictions gets write access to all product data.

## Account and admin are never granted by default

A token that can manage its user's tokens can mint a token without restrictions, and an administrator's
token can reach the instance settings. Both rights are therefore scopes of their own that a token only
receives when asked for, even when it is otherwise unrestricted. A token-authenticated request can only
create tokens that are a subset of its own token: no more scopes, no higher role cap, no further
products.

## Administrator rights are removed once, at authentication

Without the `admin` scope, an administrator's token authenticates as a copy of the user with the
administrator flag cleared. About a dozen places check that flag (the admin routes, the role bypass,
product creation, product listings); clearing it once covers all of them, including future ones, instead
of teaching each check about tokens. The role cap applies to administrators too.

## Scopes are declared per route

Every authenticated API route is registered with the scopes it needs, so a route cannot be added without
deciding that. The declarations follow one rule: every resource named in the path needs `read`, and a
state-changing method needs `write` on the first one. A test checks every route against an independent
implementation of the rule. Two cases cannot be read from the path and are checked in the services:
comments and attachments (the item they belong to is in the body or the stored row), and side effects,
which by design count as part of the request's own scope (a test run files defects with `tests:write`).

## Products and the role cap are checked where roles are

The product restriction and the role cap are applied in `Authorizer.RequireRole`, which every
product-scoped service operation already calls. Handlers that branch on the returned role (an owner may
edit, a developer may only move) see the capped role without any change. The restriction travels in the
request context, which keeps the services' signatures unchanged.

## Tokens only reach the API

Pages accept sessions only. Their CSRF protection exempts requests that are not session-authenticated,
and they are not covered by route scopes, so a token there would have bypassed both. Bearer tokens are
therefore ignored outside `/api/`.

## Restricted products fail closed

The products of a token live in their own table with a foreign key to the products. Deleting a product
removes it from every token while the token stays restricted, so a token restricted to only that product
reaches nothing rather than everything.

## Restrictions are evaluated per request

A restriction narrows the user's current rights; it is not a snapshot of them. If the user loses a role,
their tokens lose it at once, and a role cap never raises a role.
