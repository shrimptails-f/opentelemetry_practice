package main

import "go.opentelemetry.io/otel/propagation"

func propagator() propagation.TextMapPropagator { return propagation.TraceContext{} }
