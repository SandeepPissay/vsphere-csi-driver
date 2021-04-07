package opentracing

import (
	"context"
	"fmt"
	"github.com/uber/jaeger-client-go/config"
	"github.com/uber/jaeger-client-go/log"
	"github.com/opentracing/opentracing-go"
	jprom "github.com/uber/jaeger-lib/metrics/prometheus"
	cnsconfig "sigs.k8s.io/vsphere-csi-driver/pkg/common/config"
)

// global clusterid.
var clusterId string

// InitJaeger returns an instance of Jaeger Tracer that samples 100% of traces and logs all spans to stdout.
func InitJaeger(service string, csiconfig *cnsconfig.Config){
	cfg, _ := config.FromEnv()
	var err error
	tracer, _, err := cfg.NewTracer(config.Logger(log.StdLogger), config.Metrics(jprom.New()))
	if err != nil {
		panic(fmt.Sprintf("ERROR: cannot init Jaeger: %v\n", err))
	}
	opentracing.SetGlobalTracer(tracer)
	clusterId = csiconfig.Global.ClusterID
}

func StartSpan(ctx context.Context, operationName string, opts ...opentracing.StartSpanOption) (context.Context, opentracing.Span) {
	span := opentracing.StartSpan(operationName, opts...)
	span.SetTag("cluster-id", clusterId)
	return opentracing.ContextWithSpan(ctx, span), span
}
