package apic

type Logger interface {
	Info(msg string, args ...any)
}

type noLogger struct{}

func (nl noLogger) Info(_ string, _ ...any) {}
