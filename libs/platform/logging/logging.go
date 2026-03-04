package logging

import "go.uber.org/zap"

func New(serviceName string) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	logger, err := cfg.Build()
	if err != nil {
		return nil, err
	}
	return logger.With(zap.String("service", serviceName)), nil
}
