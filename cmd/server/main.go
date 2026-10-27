package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func main() {
	ctx := context.Background()
	name := os.Getenv("SERVICE_NAME")
	if name != "order-api" && name != "inventory-api" {
		log.Fatal("SERVICE_NAME must be order-api or inventory-api")
	}
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		log.Fatal(err)
	}
	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(attribute.String("service.name", name)))
	if err != nil {
		log.Fatal(err)
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(res))
	otel.SetTracerProvider(provider)
	// Explicitly propagate trace context between services.
	otel.SetTextMapPropagator(propagator())
	mux := http.NewServeMux()
	if name == "order-api" {
		mux.HandleFunc("GET /orders", order)
	} else {
		mux.HandleFunc("GET /inventory", inventory)
	}
	server := &http.Server{Addr: ":8080", Handler: otelhttp.NewHandler(mux, name), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	log.Printf("%s listening on :8080", name)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
	if err := provider.Shutdown(shutdown); err != nil {
		log.Printf("trace shutdown: %v", err)
	}
}

var client = &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport), Timeout: 4 * time.Second}

func order(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("practice/order").Start(r.Context(), "check inventory")
	defer span.End()
	mode := r.URL.Query().Get("mode")
	if mode != "" && mode != "slow" && mode != "error" {
		http.Error(w, "mode must be slow or error", http.StatusBadRequest)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://inventory-api:8080/inventory?mode="+mode, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "create request")
		http.Error(w, "request failed", 500)
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "inventory unavailable")
		http.Error(w, "inventory unavailable", 502)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		span.SetStatus(codes.Error, "inventory returned an error")
		http.Error(w, "inventory unavailable", 502)
		return
	}
	fmt.Fprintln(w, "order accepted")
}
func inventory(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("practice/inventory").Start(r.Context(), "lookup stock")
	defer span.End()
	mode := r.URL.Query().Get("mode")
	span.SetAttributes(attribute.String("practice.scenario", mode))
	if mode == "slow" {
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			span.RecordError(ctx.Err())
			span.SetStatus(codes.Error, "request cancelled")
			return
		}
	}
	if mode == "error" {
		span.SetStatus(codes.Error, "simulated stock lookup failure")
		http.Error(w, "stock lookup failed", 503)
		return
	}
	fmt.Fprintln(w, "in stock")
}
