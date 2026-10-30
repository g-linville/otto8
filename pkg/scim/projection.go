package scim

import (
	"net/url"
	"slices"
	"strings"
)

// projection selects the attributes a response returns, from the attributes and excludedAttributes query
// parameters. They apply to every response that returns a resource, as RFC 7644 section 3.9 says, including those of
// creates and updates. The id and schemas attributes are always returned.
type projection struct {
	// selected is set when the attributes parameter selects what is returned, even when it names no attribute the
	// schema defines. Then only the attributes it names are returned.
	selected   bool
	attributes []attrPath
	excluded   []attrPath
	// extensions are the URNs of the resource's schema extensions, which schemas lists only while the response holds
	// their objects.
	extensions []string
}

func parseProjection(schema *resourceSchema, q url.Values) (*projection, error) {
	attributes, err := parseAttributeList(schema, q.Get("attributes"))
	if err != nil {
		return nil, err
	}
	excluded, err := parseAttributeList(schema, q.Get("excludedAttributes"))
	if err != nil {
		return nil, err
	}
	extensions := make([]string, 0, len(schema.extensions))
	for _, ext := range schema.extensions {
		extensions = append(extensions, ext.ID)
	}
	return &projection{
		selected:   strings.TrimSpace(q.Get("attributes")) != "",
		attributes: attributes,
		excluded:   excluded,
		extensions: extensions,
	}, nil
}

// parseAttributeList parses a comma-separated list of attribute paths. Attributes that the schema does not
// define are ignored, as they would be absent from the response anyway. A path into a schema extension keeps the
// extension's URN, and other paths have none. A schema extension's URN alone names its whole object, which the
// resource holds as though it were an attribute.
func parseAttributeList(schema *resourceSchema, s string) ([]attrPath, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}

	var paths []attrPath
	for part := range strings.SplitSeq(s, ",") {
		if ext := schema.extensionSchema(strings.TrimSpace(part)); ext != nil {
			paths = append(paths, attrPath{
				Name: ext.ID,
			})
			continue
		}

		path, ok := parseAttrPath(strings.TrimSpace(part))
		if !ok {
			return nil, badRequest(scimTypeInvalidValue, "invalid attribute %q", part)
		}
		target := schema
		if ext := schema.extensionSchema(path.URN); ext != nil {
			target = ext
		}
		attr := target.attribute(path.Name)
		if attr == nil || !path.inSchema(target) {
			continue
		}
		path.URN = ""
		if target != schema {
			path.URN = target.ID
		}
		path.Name = attr.Name
		if path.Sub != "" {
			sub := attr.subAttribute(path.Sub)
			if sub == nil {
				continue
			}
			path.Sub = sub.Name
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// includes reports whether the response returns any part of the top-level attribute.
func (p *projection) includes(name string) bool {
	if p.selected {
		for _, a := range p.attributes {
			if strings.EqualFold(a.Name, name) {
				return true
			}
		}
		return false
	}
	for _, e := range p.excluded {
		if strings.EqualFold(e.Name, name) && e.Sub == "" {
			return false
		}
	}
	return true
}

func (p *projection) apply(resource map[string]any) map[string]any {
	if p.selected {
		out := map[string]any{
			"id":      resource["id"],
			"schemas": resource["schemas"],
		}
		for _, a := range p.attributes {
			from := attributesOf(resource, a.URN)
			v, ok := from[a.Name]
			if !ok {
				continue
			}
			to := out
			if a.URN != "" {
				to, _ = out[a.URN].(map[string]any)
				if to == nil {
					to = map[string]any{}
					out[a.URN] = to
				}
			}
			if a.Sub == "" {
				to[a.Name] = v
				continue
			}
			to[a.Name] = mergeSubAttribute(to[a.Name], v, a.Sub)
		}
		return p.withoutAbsentExtensions(out)
	}

	for _, e := range p.excluded {
		if e.Name == "id" || e.Name == "schemas" {
			continue
		}
		from := attributesOf(resource, e.URN)
		if e.Sub == "" {
			delete(from, e.Name)
			if e.URN != "" && len(from) == 0 {
				delete(resource, e.URN)
			}
			continue
		}
		switch v := from[e.Name].(type) {
		case map[string]any:
			delete(v, e.Sub)
		case []any:
			for _, item := range v {
				if m, ok := item.(map[string]any); ok {
					delete(m, e.Sub)
				}
			}
		}
	}
	return p.withoutAbsentExtensions(resource)
}

// withoutAbsentExtensions removes from the resource's schemas the extensions whose objects the projection left out.
func (p *projection) withoutAbsentExtensions(resource map[string]any) map[string]any {
	schemas, ok := resource["schemas"].([]any)
	if !ok {
		return resource
	}
	kept := make([]any, 0, len(schemas))
	for _, schema := range schemas {
		if urn, _ := schema.(string); slices.Contains(p.extensions, urn) {
			if _, present := resource[urn]; !present {
				continue
			}
		}
		kept = append(kept, schema)
	}
	resource["schemas"] = kept
	return resource
}

// attributesOf returns the object that holds the attributes of the schema extension with the given URN, or the
// resource itself when the URN is empty. It is nil when the resource has none of the extension's attributes.
func attributesOf(resource map[string]any, urn string) map[string]any {
	if urn == "" {
		return resource
	}
	m, _ := resource[urn].(map[string]any)
	return m
}

// mergeSubAttribute adds the sub-attribute of value, a complex or multi-valued complex attribute, to projected.
func mergeSubAttribute(projected, value any, sub string) any {
	switch v := value.(type) {
	case map[string]any:
		out, _ := projected.(map[string]any)
		if out == nil {
			out = map[string]any{}
		}
		if s, ok := v[sub]; ok {
			out[sub] = s
		}
		return out
	case []any:
		out, _ := projected.([]any)
		if out == nil {
			out = make([]any, len(v))
			for i := range out {
				out[i] = map[string]any{}
			}
		}
		for i, item := range v {
			if m, ok := item.(map[string]any); ok {
				if s, ok := m[sub]; ok {
					out[i].(map[string]any)[sub] = s
				}
			}
		}
		return out
	}
	return projected
}
