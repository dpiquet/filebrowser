package api

// adminHandlers provides router for all requests available for admin users only
type adminHandlers struct {
	store adminStore
}

type adminStore interface {
}
