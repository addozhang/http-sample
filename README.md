## Base Application Configuration

### Environment Variables

- app: Specifies the name of the current service. For example, when deploying service-a, this value should be set to service-a. 
- version: Sets the version of the current service. In this example, all services use v1 as their version number.
- upstream: Specifies the URL of the downstream service which will send request to. For example, service-a can set upstream 
to http://service-b:8080/, and so forth. If emitted, it will respond directly.

### Port Configuration
The application listens on port 8080. Ensure that the containerPort in the Kubernetes deployment configuration is set to 8080.

### Example Configuration

For service-a, the configuration should look like this:

```yaml
containers:
- name: service-a
  image: addozhang/http-sample
  imagePullPolicy: Always
  ports:
    - containerPort: 8080
      env:
    - name: app
      value: "service-a"
    - name: version
      value: "v1"
    - name: upstream
      value: "http://service-b:8080/"
```

## Application Behavior

When `service-a` is accessed, it forwards the request to `service-b` as configured in the `upstream` 
environment variable, and `service-b` further forwards it to `service-c`. Each service, while forwarding 
the request, appends its information (version, IP, and hostname).

You might receive response like below:

```
service-a(version: v1, ip: 10.42.0.75, hostname: service-a-74d6b8d9cd-n84h2) -> service-b(version: v1, ip: 10.42.0.74, hostname: service-b-859ff9bb95-7f6xf) -> service-c(version: v1, ip: 10.42.0.73, hostname: service-c-84bb4bcfcc-wkjhp)
```

## OpenTelemetry Configuration

The application incorporates manual instrumentation using the OpenTelemetry SDK.

### Environment Variables

* `OTEL_EXPORTER_OTLP_ENDPOINT`: This specifies the OTLP endpoint for the OpenTelemetry collector. If this variable is not set, the SDK will not be activated. For instance, use `localhost:4318`. It's important to note that the scheme is not required in this setting.
* `OTEL_PROPAGATORS`: This variable determines the propagator mode within the context. Options include `tracecontext`, `baggage`, `b3`, `b3multi`, or `jaeger`. Each option specifies a different propagation format for distributed tracing.
