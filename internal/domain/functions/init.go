package functions

import (
	"context"
	"os"

	"github.com/KHU-RETURN/rcp-server/ent"
)

func Init(ctx context.Context, client *ent.Client) (*Handler, error) {
	svc, err := NewService(ctx, NewRepository(client))
	if err != nil {
		return nil, err
	}
	builder, err := BuilderFromEnv()
	if err != nil {
		_ = svc.Close(ctx)
		return nil, err
	}
	svc.SetBuilder(builder)
	dir := os.Getenv("RCP_FUNCTION_DATA_DIR")
	if dir == "" {
		dir = "function-data"
	}
	data, err := OpenDataStore(dir)
	if err != nil {
		_ = svc.Close(ctx)
		return nil, err
	}
	svc.SetDataStore(data)
	return NewHandler(svc), nil
}
