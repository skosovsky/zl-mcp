package main

import "time"

// Native bounds and defaults retain the established service contract.
const (
	defaultCaseBudget = 60 * time.Second
	modelCaseBudget   = 180 * time.Second
)
