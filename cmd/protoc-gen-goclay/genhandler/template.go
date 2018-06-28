package genhandler

import (
	"bytes"
	"strings"
	"text/template"

	pbdescriptor "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/golang/protobuf/protoc-gen-go/generator"
	"github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway/descriptor"
	"github.com/pkg/errors"
)

var (
	errNoTargetService = errors.New("no target service defined in the file")
)

var pkg map[string]string

type param struct {
	*descriptor.File
	Imports          []descriptor.GoPackage
	SwaggerBuffer    []byte
	ApplyMiddlewares bool
	CurrentPath      string
}

func applyImplTemplate(p param) (string, error) {
	w := bytes.NewBuffer(nil)

	if err := implTemplate.Execute(w, p); err != nil {
		return "", err
	}

	return w.String(), nil
}

func applyDescTemplate(p param) (string, error) {
	// r := &http.Request{}
	// r.URL.Query()
	w := bytes.NewBuffer(nil)
	if err := headerTemplate.Execute(w, p); err != nil {
		return "", err
	}

	if err := regTemplate.ExecuteTemplate(w, "base", p); err != nil {
		return "", err
	}

	if err := clientTemplate.Execute(w, p); err != nil {
		return "", err
	}

	if err := marshalersTemplate.Execute(w, p); err != nil {
		return "", err
	}

	if err := patternsTemplate.ExecuteTemplate(w, "base", p); err != nil {
		return "", err
	}

	if p.SwaggerBuffer != nil {
		if err := footerTemplate.Execute(w, p); err != nil {
			return "", err
		}
	}

	return w.String(), nil
}

var (
	varNameReplacer = strings.NewReplacer(
		".", "_",
		"/", "_",
		"-", "_",
	)
	funcMap = template.FuncMap{
		"hasAsterisk": func(ss []string) bool {
			for _, s := range ss {
				if s == "*" {
					return true
				}
			}
			return false
		},
		"varName": func(s string) string { return varNameReplacer.Replace(s) },
		"goTypeName": func(s string) string {
			toks := strings.Split(s, ".")
			for pos := range toks {
				toks[pos] = generator.CamelCase(toks[pos])
			}
			return strings.Join(toks, ".")
		},
		"byteStr":         func(b []byte) string { return string(b) },
		"escapeBackTicks": func(s string) string { return strings.Replace(s, "`", "` + \"``\" + `", -1) },
		"toGoType":        func(t pbdescriptor.FieldDescriptorProto_Type) string { return primitiveTypeToGo(t) },
		// arrayToPathInterp replaces chi-style path to fmt.Sprint-style path.
		"arrayToPathInterp": func(tpl string) string {
			vv := strings.Split(tpl, "/")
			ret := []string{}
			for _, v := range vv {
				if strings.HasPrefix(v, "{") {
					ret = append(ret, "%v")
					continue
				}
				ret = append(ret, v)
			}
			return strings.Join(ret, "/")
		},
		// returns safe package prefix with dot(.) or empty string by imported package name or alias
		"pkg": func(name string) string {
			if p, ok := pkg[name]; ok && p != "" {
				return p + "."
			}
			return ""
		},
		"hasBindings": hasBindings,
	}

	headerTemplate = template.Must(template.New("header").Funcs(funcMap).Parse(`
// Code generated by protoc-gen-goclay
// source: {{ .GetName }}
// DO NOT EDIT!

/*
Package {{ .GoPkg.Name }} is a self-registering gRPC and JSON+Swagger service definition.

It conforms to the github.com/utrack/clay/v2/transport Service interface.
*/
package {{ .GoPkg.Name }}
import (
    {{ range $i := .Imports }}{{ if $i.Standard }}{{ $i | printf "%s\n" }}{{ end }}{{ end }}

    {{ range $i := .Imports }}{{ if not $i.Standard }}{{ $i | printf "%s\n" }}{{ end }}{{ end }}
)

// Update your shared lib or downgrade generator to v1 if there's an error
var _ = {{ pkg "transport" }}IsVersion2

var _ = {{ pkg "ioutil" }}Discard
var _ {{ pkg "chi" }}Router
var _ {{ pkg "runtime" }}Marshaler
var _ {{ pkg "bytes" }}Buffer
var _ {{ pkg "context" }}Context
var _ {{ pkg "fmt" }}Formatter
var _ {{ pkg "strings" }}Reader
var _ {{ pkg "errors" }}Frame
var _ {{ pkg "httpruntime" }}Marshaler
var _ {{ pkg "http" }}Handler
`))

	footerTemplate = template.Must(template.New("footer").Funcs(funcMap).Parse(`
    var _swaggerDef_{{ varName .GetName }} = []byte(` + "`" + `{{ escapeBackTicks (byteStr .SwaggerBuffer) }}` + `
` + "`)" + `
`))

	marshalersTemplate = template.Must(template.New("patterns").Funcs(funcMap).Parse(`
{{ range $svc := .Services }}
// patterns for {{ $svc.GetName }}
var (
{{ range $m := $svc.Methods }}
{{ range $b := $m.Bindings }}

    pattern_goclay_{{ $svc.GetName }}_{{ $m.GetName }}_{{ $b.Index }} = "{{ $b.PathTmpl.Template }}"

    pattern_goclay_{{ $svc.GetName }}_{{ $m.GetName }}_{{ $b.Index }}_builder = func(
        {{ range $p := $b.PathParams -}}
            {{ $p.Target.GetName }} {{ toGoType $p.Target.GetType }},
        {{ end -}}
    ) string {
        return {{ pkg "fmt" }}Sprintf("{{ arrayToPathInterp $b.PathTmpl.Template }}",{{ range $p := $b.PathParams }}{{ $p.Target.GetName }},{{ end }})
    }

    {{ if not (hasAsterisk $b.ExplicitParams) }}
        unmarshaler_goclay_{{ $svc.GetName }}_{{ $m.GetName }}_{{ $b.Index }}_boundParams = map[string]struct{}{
            {{ range $n := $b.ExplicitParams -}}
                "{{ $n }}": struct{}{},
            {{ end }}
        }
    {{ end }}
{{ end }}
{{ end }}
)
{{ end }}
`))

	patternsTemplate = template.Must(template.New("patterns").Funcs(funcMap).Parse(`
{{ define "base" }}
{{ range $svc := .Services }}
// marshalers for {{ $svc.GetName }}
var (
{{ range $m := $svc.Methods }}
{{ range $b := $m.Bindings }}

    unmarshaler_goclay_{{ $svc.GetName }}_{{ $m.GetName }}_{{ $b.Index }} = func(r *{{ pkg "http" }}Request) (*{{$m.RequestType.GoType $m.Service.File.GoPkg.Path }},error) {
	var req {{$m.RequestType.GoType $m.Service.File.GoPkg.Path }}

        {{ if not (hasAsterisk $b.ExplicitParams) }}
            for k,v := range r.URL.Query() {
                if _,ok := unmarshaler_goclay_{{ $svc.GetName }}_{{ $m.GetName }}_{{ $b.Index }}_boundParams[{{ pkg "strings" }}ToLower(k)];ok {
                    continue
                }
                if err := {{ pkg "errors" }}Wrap({{ pkg "runtime" }}PopulateFieldFromPath(&req, k, v[0]), "couldn't populate field from Path"); err != nil {
                    return nil, {{ pkg "httpruntime" }}TransformUnmarshalerError(err)
                }
            }
        {{ end }}
        {{- if $b.Body -}}
            {{- template "unmbody" . -}}
        {{- end -}}
        {{- if $b.PathParams -}}
            {{- template "unmpath" . -}}
        {{ end }}
        return &req, nil
    }
{{ end }}
{{ end }}
{{ end }}
)
{{ end }}
{{ define "unmbody" }}
    inbound,_ := {{ pkg "httpruntime" }}MarshalerForRequest(r)
    if err := {{ pkg "errors" }}Wrap(inbound.Unmarshal(r.Body,&{{.Body.AssignableExpr "req"}}),"couldn't read request JSON"); err != nil {
        return nil, {{ pkg "httpruntime" }}TransformUnmarshalerError(err)
    }
{{ end }}
{{ define "unmpath" }}
    rctx := {{ pkg "chi" }}RouteContext(r.Context())
    if rctx == nil {
        panic("Only chi router is supported for GETs atm")
    }
    for pos,k := range rctx.URLParams.Keys {
        if err := {{ pkg "errors" }}Wrap({{ pkg "runtime" }}PopulateFieldFromPath(&req, k, rctx.URLParams.Values[pos]), "couldn't populate field from Path"); err != nil {
            return nil, {{ pkg "httpruntime" }}TransformUnmarshalerError(err)
        }
    }
{{ end }}
`))

	implTemplate = template.Must(template.New("impl").Funcs(funcMap).Parse(`
// Code generated by protoc-gen-goclay, but your can (must) modify it.
// source: {{ .GetName }}

package  {{ .GoPkg.Name }}

import (
    {{ range $i := .Imports }}{{ if $i.Standard }}{{ $i | printf "%s\n" }}{{ end }}{{ end }}

    {{ range $i := .Imports }}{{ if not $i.Standard }}{{ $i | printf "%s\n" }}{{ end }}{{ end }}
)

{{ range $service := .Services }}

type {{ $service.GetName }}Implementation struct {}

func New{{ $service.GetName }}() *{{ $service.GetName }}Implementation {
    return &{{ $service.GetName }}Implementation{}
}

{{ range $method := $service.Methods }}
func (i *{{ $service.GetName }}Implementation) {{ $method.Name }}(ctx {{ pkg "context" }}Context, req *{{ $method.RequestType.GoType $.CurrentPath }}) (*{{ $method.ResponseType.GoType $.CurrentPath }}, error) {
    return nil, {{ pkg "errors" }}New("not implemented")
}
{{ end }}

// GetDescription is a simple alias to the ServiceDesc constructor.
// It makes it possible to register the service implementation @ the server.
func (i *{{ $service.GetName }}Implementation) GetDescription() {{ pkg "transport" }}ServiceDesc {
    return {{ pkg "desc" }}New{{ $service.GetName }}ServiceDesc(i)
}

{{ end }}
`))
)
