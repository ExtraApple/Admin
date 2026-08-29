package application

// RuntimeLogField is a provider-neutral, allowlisted runtime-log dimension.
// It intentionally has no error field: raw infrastructure errors must not
// cross the Messaging runtime logging contract.
type RuntimeLogField struct {
	Key   string
	Value any
}

type RuntimeLogger interface {
	Info(string, ...RuntimeLogField)
	Warn(string, ...RuntimeLogField)
}

func runtimeField(key string, value any) RuntimeLogField {
	return RuntimeLogField{Key: key, Value: value}
}

type discardRuntimeLogger struct{}

func (discardRuntimeLogger) Info(string, ...RuntimeLogField) {}
func (discardRuntimeLogger) Warn(string, ...RuntimeLogField) {}
