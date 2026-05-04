package supergo

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

// OpenAPISpec is a parsed OpenAPI document that can be reused across requests.
type OpenAPISpec struct {
	doc  openAPIDocument
	path string
}

// LoadOpenAPISpec reads and parses an OpenAPI spec from disk.
func LoadOpenAPISpec(path string) (*OpenAPISpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading OpenAPI spec %q: %w", path, err)
	}

	var doc openAPIDocument
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing OpenAPI spec %q: %w", path, err)
	}
	if len(doc.Paths) == 0 {
		return nil, fmt.Errorf("OpenAPI spec %q has no paths", path)
	}

	return &OpenAPISpec{doc: doc, path: path}, nil
}

// MustOpenAPISpec loads an OpenAPI spec or panics.
func MustOpenAPISpec(path string) *OpenAPISpec {
	spec, err := LoadOpenAPISpec(path)
	if err != nil {
		panic(err)
	}
	return spec
}

func (s *OpenAPISpec) validateResponse(method, rawPath string, res *Response) error {
	if s == nil {
		return fmt.Errorf("OpenAPI spec is nil")
	}

	op, template, err := s.findOperation(method, rawPath)
	if err != nil {
		return err
	}

	specResp, code, err := op.findResponse(res.StatusCode)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, template, err)
	}
	if len(specResp.Content) == 0 {
		return nil
	}

	actualContentType, _, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if err != nil {
		actualContentType = res.Header.Get("Content-Type")
	}
	mediaType, mediaTypeName := findMatchingMediaType(specResp.Content, actualContentType)
	if mediaType == nil {
		return fmt.Errorf("%s %s: response status %s content-type %q not declared in spec", method, template, code, actualContentType)
	}
	if mediaType.Schema == nil {
		return nil
	}
	if len(res.Body) == 0 {
		return fmt.Errorf("%s %s: response status %s expects body matching %s, got empty body", method, template, code, mediaTypeName)
	}

	var body any
	if err := json.Unmarshal(res.Body, &body); err != nil {
		return fmt.Errorf("%s %s: response body is not valid JSON: %w", method, template, err)
	}

	schema, err := s.resolveSchema(mediaType.Schema)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, template, err)
	}
	if err := s.validateValue(body, schema, "$"); err != nil {
		return fmt.Errorf("%s %s: response body failed schema validation: %w", method, template, err)
	}

	return nil
}

func (s *OpenAPISpec) findOperation(method, rawPath string) (*openAPIOperation, string, error) {
	actualPath := normalizeRequestPath(rawPath)
	var bestOp *openAPIOperation
	var bestTemplate string
	bestScore := -1

	for template, item := range s.doc.Paths {
		score, ok := matchPathTemplate(template, actualPath)
		if !ok || score < bestScore {
			continue
		}
		op := item.operation(method)
		if op == nil {
			continue
		}
		bestOp = op
		bestTemplate = template
		bestScore = score
	}

	if bestOp == nil {
		return nil, "", fmt.Errorf("no OpenAPI operation found for %s %s", method, actualPath)
	}
	return bestOp, bestTemplate, nil
}

func (s *OpenAPISpec) resolveSchema(ref *openAPISchemaRef) (*openAPISchemaRef, error) {
	if ref == nil {
		return nil, fmt.Errorf("schema is nil")
	}
	if ref.Ref == "" {
		return ref, nil
	}

	const prefix = "#/components/schemas/"
	if !strings.HasPrefix(ref.Ref, prefix) {
		return nil, fmt.Errorf("unsupported schema ref %q", ref.Ref)
	}
	name := strings.TrimPrefix(ref.Ref, prefix)
	target, ok := s.doc.Components.Schemas[name]
	if !ok {
		return nil, fmt.Errorf("schema ref %q not found", ref.Ref)
	}
	return s.resolveSchema(target)
}

func (s *OpenAPISpec) validateValue(value any, schema *openAPISchemaRef, at string) error {
	resolved, err := s.resolveSchema(schema)
	if err != nil {
		return err
	}

	switch resolved.Type {
	case "object":
		obj, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: expected object, got %T", at, value)
		}
		for _, key := range resolved.Required {
			if _, ok := obj[key]; !ok {
				return fmt.Errorf("%s: missing required property %q", at, key)
			}
		}
		if isAdditionalPropertiesFalse(resolved.AdditionalProperties) {
			for key := range obj {
				if _, ok := resolved.Properties[key]; !ok {
					return fmt.Errorf("%s: unexpected property %q", at, key)
				}
			}
		}
		for key, child := range resolved.Properties {
			got, ok := obj[key]
			if !ok {
				continue
			}
			if err := s.validateValue(got, child, at+"."+key); err != nil {
				return err
			}
		}
		return nil
	case "array":
		arr, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s: expected array, got %T", at, value)
		}
		if resolved.Items == nil {
			return nil
		}
		for i, item := range arr {
			if err := s.validateValue(item, resolved.Items, fmt.Sprintf("%s[%d]", at, i)); err != nil {
				return err
			}
		}
		return nil
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s: expected string, got %T", at, value)
		}
		return nil
	case "integer":
		num, ok := value.(float64)
		if !ok || num != float64(int64(num)) {
			return fmt.Errorf("%s: expected integer, got %T", at, value)
		}
		return nil
	case "number":
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("%s: expected number, got %T", at, value)
		}
		return nil
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s: expected boolean, got %T", at, value)
		}
		return nil
	case "":
		return nil
	default:
		return fmt.Errorf("%s: unsupported schema type %q", at, resolved.Type)
	}
}

func normalizeRequestPath(rawPath string) string {
	if rawPath == "" {
		return "/"
	}
	u, err := url.ParseRequestURI(rawPath)
	if err == nil && u.Path != "" {
		return path.Clean(u.Path)
	}
	return path.Clean(rawPath)
}

func matchPathTemplate(template, actual string) (int, bool) {
	template = path.Clean(template)
	actual = path.Clean(actual)
	if template == actual {
		return 1_000_000, true
	}

	templateParts := splitPath(template)
	actualParts := splitPath(actual)
	if len(templateParts) != len(actualParts) {
		return 0, false
	}

	score := 0
	for i := range templateParts {
		tp := templateParts[i]
		ap := actualParts[i]
		if isPathParam(tp) {
			if ap == "" {
				return 0, false
			}
			score++
			continue
		}
		if tp != ap {
			return 0, false
		}
		score += 10
	}
	return score, true
}

func splitPath(p string) []string {
	p = strings.Trim(path.Clean(p), "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func isPathParam(segment string) bool {
	return strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}")
}

func findMatchingMediaType(content map[string]*openAPIMediaType, actual string) (*openAPIMediaType, string) {
	if mediaType, ok := content[actual]; ok {
		return mediaType, actual
	}
	if strings.HasSuffix(actual, "+json") {
		if mediaType, ok := content["application/json"]; ok {
			return mediaType, "application/json"
		}
	}
	return nil, ""
}

func isAdditionalPropertiesFalse(v any) bool {
	b, ok := v.(bool)
	return ok && !b
}

type openAPIDocument struct {
	Paths      map[string]*openAPIPathItem `yaml:"paths"`
	Components openAPIComponents          `yaml:"components"`
}

type openAPIComponents struct {
	Schemas map[string]*openAPISchemaRef `yaml:"schemas"`
}

type openAPIPathItem struct {
	Get     *openAPIOperation `yaml:"get"`
	Post    *openAPIOperation `yaml:"post"`
	Put     *openAPIOperation `yaml:"put"`
	Patch   *openAPIOperation `yaml:"patch"`
	Delete  *openAPIOperation `yaml:"delete"`
	Head    *openAPIOperation `yaml:"head"`
	Options *openAPIOperation `yaml:"options"`
}

func (p *openAPIPathItem) operation(method string) *openAPIOperation {
	switch strings.ToUpper(method) {
	case "GET":
		return p.Get
	case "POST":
		return p.Post
	case "PUT":
		return p.Put
	case "PATCH":
		return p.Patch
	case "DELETE":
		return p.Delete
	case "HEAD":
		return p.Head
	case "OPTIONS":
		return p.Options
	default:
		return nil
	}
}

type openAPIOperation struct {
	Responses map[string]*openAPIResponse `yaml:"responses"`
}

func (o *openAPIOperation) findResponse(statusCode int) (*openAPIResponse, string, error) {
	if o == nil {
		return nil, "", fmt.Errorf("operation is nil")
	}
	code := strconv.Itoa(statusCode)
	if resp, ok := o.Responses[code]; ok {
		return resp, code, nil
	}
	if resp, ok := o.Responses["default"]; ok {
		return resp, "default", nil
	}
	return nil, "", fmt.Errorf("response status %d not declared in spec", statusCode)
}

type openAPIResponse struct {
	Content map[string]*openAPIMediaType `yaml:"content"`
}

type openAPIMediaType struct {
	Schema *openAPISchemaRef `yaml:"schema"`
}

type openAPISchemaRef struct {
	Ref                  string                       `yaml:"$ref"`
	Type                 string                       `yaml:"type"`
	Required             []string                     `yaml:"required"`
	Properties           map[string]*openAPISchemaRef `yaml:"properties"`
	Items                *openAPISchemaRef           `yaml:"items"`
	AdditionalProperties any                          `yaml:"additionalProperties"`
}
