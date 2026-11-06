package cliinput

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	flowschema "github.com/nexssp/flow/extensions/schema"
	"github.com/nexssp/kernel/action"
	"github.com/nexssp/transport/tcli"
	tcliflow "github.com/nexssp/transport/tcli/nexssflow"
)

type (
	rawArgumentsKey struct{}
	inputSchemasKey struct{}
	commandLineKey  struct{}
)

// WithCommandLine stores the canonical executable name and original argv for a
// single run so reports can show how the command was invoked.
func WithCommandLine(ctx context.Context, executable string, args []string) context.Context {
	parts := make([]string, 1, len(args)+1)
	parts[0] = shellQuote(executable)
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return context.WithValue(ctx, commandLineKey{}, strings.Join(parts, " "))
}

// CommandLine returns the invocation attached to ctx, or an empty string when
// the caller did not provide one.
func CommandLine(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	line, _ := ctx.Value(commandLineKey{}).(string)
	return line
}

func shellWordCharacter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("_-./:=,+@%", r)
}

func shellQuote(value string) string {
	var clean strings.Builder
	for _, r := range value {
		if unicode.IsControl(r) {
			_, _ = fmt.Fprintf(&clean, "\\u%04X", r)
			continue
		}
		clean.WriteRune(r)
	}
	value = clean.String()
	if value != "" {
		plain := true
		for _, r := range value {
			if !shellWordCharacter(r) {
				plain = false
				break
			}
		}
		if plain {
			return value
		}
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// FieldValue is one raw CLI value attributed to a request field. Index is the
// argv index of the value token (or the flag token for an inline/implicit bool).
type FieldValue struct {
	Field string
	Value string
	Index int
}

// WithRawArguments attaches the command's original argv (without its command
// name) to a single invocation. The slice is copied so later caller mutation
// cannot affect observation.
func WithRawArguments(ctx context.Context, args []string) context.Context {
	return context.WithValue(ctx, rawArgumentsKey{}, append([]string(nil), args...))
}

// WithInputSchemas attaches the CLI request schemas applicable to an
// invocation. It is deliberately independent of tcli's parsing/binding path.
func WithInputSchemas(ctx context.Context, schemas map[string]tcli.CLIRequestSchema) context.Context {
	copyOfSchemas := make(map[string]tcli.CLIRequestSchema, len(schemas))
	for name, schema := range schemas {
		schema.Fields = append([]tcli.CLIFieldSpec(nil), schema.Fields...)
		for i := range schema.Fields {
			schema.Fields[i].Flags = append([]string(nil), schema.Fields[i].Flags...)
		}
		copyOfSchemas[name] = schema
	}
	return context.WithValue(ctx, inputSchemasKey{}, copyOfSchemas)
}

// OrderedFieldValues reports selected fields in their original argv order.
// It observes lexical boundaries only; tcli remains responsible for parsing,
// conversion and validation. Unknown or otherwise invalid arguments are not
// errors here because tcli will reject them before invoking a domain action.
func OrderedFieldValues(ctx context.Context, schemaName string, fields ...string) []FieldValue {
	args, hasArgs := ctx.Value(rawArgumentsKey{}).([]string)
	schemas, hasSchemas := ctx.Value(inputSchemasKey{}).(map[string]tcli.CLIRequestSchema)
	if !hasArgs || !hasSchemas {
		return nil
	}
	schema, ok := schemas[schemaName]
	if !ok {
		return nil
	}
	selected := make(map[string]bool, len(fields))
	for _, field := range fields {
		selected[field] = true
	}
	if len(selected) == 0 {
		return nil
	}
	fieldByFlag, positionalFields := indexSchemaFields(schema.Fields)
	observed, positionals := scanArgumentValues(args, fieldByFlag, selected)
	observed = appendPositionalValues(observed, positionals, positionalFields, selected)
	sortFieldValues(observed)
	inv, _ := ctx.Value(invocationKey{}).(invocation)
	for i := range observed {
		observed[i].Value = restoreString(observed[i].Value, inv.marker)
	}
	return observed
}

func indexSchemaFields(fields []tcli.CLIFieldSpec) (map[string]tcli.CLIFieldSpec, []tcli.CLIFieldSpec) {
	fieldByFlag := make(map[string]tcli.CLIFieldSpec)
	var positionalFields []tcli.CLIFieldSpec
	for _, field := range fields {
		if field.Positional {
			positionalFields = append(positionalFields, field)
			continue
		}
		aliases := append([]string(nil), field.Flags...)
		foundName := false
		for _, alias := range aliases {
			if normalizeAlias(alias) == field.Name {
				foundName = true
				break
			}
		}
		if !foundName {
			aliases = append(aliases, field.Name)
		}
		for _, alias := range aliases {
			alias = normalizeAlias(alias)
			if alias != "" {
				fieldByFlag[alias] = field
			}
		}
	}
	return fieldByFlag, positionalFields
}

func scanArgumentValues(args []string, fieldByFlag map[string]tcli.CLIFieldSpec, selected map[string]bool) ([]FieldValue, []FieldValue) {
	var observed, positionals []FieldValue
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			for j := i + 1; j < len(args); j++ {
				positionals = append(positionals, FieldValue{Value: args[j], Index: j})
			}
			break
		}
		key, inlineValue, hasInlineValue, isFlag := splitCLIFlag(arg)
		if !isFlag {
			positionals = append(positionals, FieldValue{Value: arg, Index: i})
			continue
		}
		field, known := fieldByFlag[key]
		if !known {
			continue
		}
		value, valueIndex, consumed := observedFlagValue(args, i, inlineValue, hasInlineValue, field.Type)
		if consumed {
			i++
		}
		if selected[field.Name] {
			observed = append(observed, FieldValue{Field: field.Name, Value: value, Index: valueIndex})
		}
	}
	return observed, positionals
}

func appendPositionalValues(observed, positionals []FieldValue, positionalFields []tcli.CLIFieldSpec, selected map[string]bool) []FieldValue {
	positionalIndex := 0
	for _, value := range positionals {
		if positionalIndex >= len(positionalFields) {
			break
		}
		field := positionalFields[positionalIndex]
		if selected[field.Name] {
			observed = append(observed, FieldValue{Field: field.Name, Value: value.Value, Index: value.Index})
		}
		if !isSliceType(field.Type) {
			positionalIndex++
		}
	}
	return observed
}

func sortFieldValues(values []FieldValue) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j].Index < values[j-1].Index; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func normalizeAlias(alias string) string {
	return strings.TrimLeft(strings.TrimSpace(alias), "-")
}

func splitCLIFlag(arg string) (key, value string, hasValue, isFlag bool) {
	switch {
	case strings.HasPrefix(arg, "--"):
		body := arg[2:]
		key, value, hasValue = strings.Cut(body, "=")
		return key, value, hasValue, true
	case strings.HasPrefix(arg, "-") && len(arg) > 1:
		body := arg[1:]
		key, value, hasValue = strings.Cut(body, "=")
		return key, value, hasValue, true
	default:
		return "", "", false, false
	}
}

func observedFlagValue(args []string, index int, inlineValue string, hasInlineValue bool, typ string) (value string, valueIndex int, consumed bool) {
	if hasInlineValue && inlineValue != "" {
		return inlineValue, index, false
	}
	if isBoolType(typ) {
		if index+1 < len(args) && (args[index+1] == "true" || args[index+1] == "false") {
			return args[index+1], index + 1, true
		}
		return "true", index, false
	}
	if index+1 < len(args) && canBeSeparateValue(args[index+1]) {
		return args[index+1], index + 1, true
	}
	// Missing/invalid values are intentionally not diagnosed by the observer.
	return "", index, false
}

func canBeSeparateValue(arg string) bool {
	if !strings.HasPrefix(arg, "-") {
		return true
	}
	return len(arg) >= 2 && (arg[1] == '.' || (arg[1] >= '0' && arg[1] <= '9'))
}

func isBoolType(raw string) bool {
	typ := strings.TrimSpace(raw)
	for strings.HasPrefix(typ, "*") {
		typ = strings.TrimSpace(typ[1:])
	}
	return typ == "bool"
}

func isSliceType(raw string) bool {
	typ := strings.TrimSpace(raw)
	for strings.HasPrefix(typ, "*") {
		typ = strings.TrimSpace(typ[1:])
	}
	return strings.HasPrefix(typ, "[]")
}

// WrapInputSchemas adapts Flow @schema metadata into tcli request schemas and
// makes those schemas available to downstream actions without hard-coding an
// input schema or changing the request passed through the action.
func WrapInputSchemas(meta map[string]any, inner action.AnyAction) (action.AnyAction, error) {
	requestSchemas := make(map[string]tcli.CLIRequestSchema)
	for _, schema := range flowschema.SchemasFromMap(meta) {
		hasCLIFields := false
		for i := range schema.Fields {
			field := &schema.Fields[i]
			if strings.TrimSpace(field.Tags["cli"]) != "" {
				hasCLIFields = true
				break
			}
		}
		if !hasCLIFields {
			continue
		}
		requestSchema, err := tcliflow.CLIRequestSchemaFromFlowSchema(schema)
		if err != nil {
			return nil, err
		}
		requestSchemas[schema.Name] = requestSchema
	}
	if len(requestSchemas) == 0 {
		return inner, nil
	}
	return action.Dynamic(inner).Use(func(next action.Fn[any, any]) action.Fn[any, any] {
		return func(ctx context.Context, req any) (any, error) {
			ctx = WithInputSchemas(ctx, requestSchemas)
			return next(ctx, req)
		}
	}).Build(), nil
}
