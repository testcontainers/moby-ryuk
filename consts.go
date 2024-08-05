package main

const (
	// labelBase is the base label for testcontainers.
	labelBase = "org.testcontainers"

	// ryukLabel is the label used to identify reaper containers.
	ryukLabel = labelBase + ".ryuk"

	// fieldError is the log field key for errors.
	fieldError = "error"

	// fieldAddress is the log field a client or listening address.
	fieldAddress = "address"

	// fieldClients is the log field used for client counts.
	fieldClients = "clients"
)
