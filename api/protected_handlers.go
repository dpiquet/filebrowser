package api

// protectedHandlers provides router for all requests available for regular users
type protectedHandlers struct {
	store protectedStore
}

type protectedStore interface {
}
