package e2eframe

type UnitKind string

// IsValid checks if a service kind is registered.
func (k UnitKind) IsValid() bool {
	return KindExists(k)
}

// KindExists checks if a service kind is registered.
func KindExists(kind UnitKind) bool {
	_, ok := unitMarshallerRegistry[kind]

	return ok
}
