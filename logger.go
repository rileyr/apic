package apic

type Logger interface {
	Info(msg string, args ...any)
	Debug(msg string, args ...any)
}

type noLogger struct{}

func (nl noLogger) Info(_ string, _ ...any)  {}
func (nl noLogger) Debug(_ string, _ ...any) {}
