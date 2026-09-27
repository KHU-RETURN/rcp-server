package functions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/google/uuid"
)

const maxDataOperationsPerInvocation = 32

type dataInput struct {
	ctx     context.Context
	replies chan []byte
	current *bytes.Reader
}

func (in *dataInput) Read(p []byte) (int, error) {
	for {
		if in.current != nil && in.current.Len() > 0 {
			return in.current.Read(p)
		}
		select {
		case reply := <-in.replies:
			in.current = bytes.NewReader(reply)
		case <-in.ctx.Done():
			return 0, io.EOF
		}
	}
}

type dataRequest struct {
	Type       string          `json:"$rcp"`
	Op         string          `json:"op"`
	Collection string          `json:"collection"`
	Key        string          `json:"key"`
	Value      json.RawMessage `json:"value"`
	Offset     int             `json:"offset"`
	Binding    string          `json:"binding"`
	SQL        string          `json:"sql"`
	Params     []any           `json:"params"`
}

type dataReply struct {
	Type         string           `json:"$rcp"`
	OK           bool             `json:"ok"`
	Item         *DataItem        `json:"item,omitempty"`
	Items        []DataItem       `json:"items,omitempty"`
	Columns      []string         `json:"columns,omitempty"`
	Rows         []map[string]any `json:"rows,omitempty"`
	RowsAffected int64            `json:"rows_affected,omitempty"`
	Error        string           `json:"error,omitempty"`
}

type dataOutput struct {
	ctx      context.Context
	store    *DataStore
	owner    uuid.UUID
	function uuid.UUID
	input    *dataInput
	final    limitedWriter
	pending  []byte
	total    int
	ops      int
	exceeded bool
}

func (out *dataOutput) Write(p []byte) (int, error) {
	if out.total+len(p) > MaxOutputBytes {
		out.exceeded = true
		return 0, ErrOutputLimit
	}
	out.total += len(p)
	out.pending = append(out.pending, p...)
	for {
		index := bytes.IndexByte(out.pending, '\n')
		if index < 0 {
			return len(p), nil
		}
		line := bytes.TrimSpace(out.pending[:index])
		out.pending = out.pending[index+1:]
		if len(line) == 0 {
			continue
		}
		if err := out.line(line); err != nil {
			return 0, err
		}
	}
}

func (out *dataOutput) line(line []byte) error {
	var req dataRequest
	if json.Unmarshal(line, &req) == nil && (req.Type == "db" || req.Type == "sql") {
		out.ops++
		reply := dataReply{Type: req.Type + ".result"}
		if out.ops > maxDataOperationsPerInvocation {
			reply.Error = "function data operation limit reached"
		} else {
			var err error
			if req.Type == "sql" {
				var result *SQLResult
				result, err = out.store.QueryBinding(out.ctx, out.owner, out.function, req.Binding, req.SQL, req.Params)
				if err == nil {
					reply.Columns, reply.Rows, reply.RowsAffected = result.Columns, result.Rows, result.RowsAffected
				}
			} else {
				switch req.Op {
				case "get":
					reply.Item, err = out.store.Get(out.ctx, out.owner, out.function, req.Collection, req.Key)
				case "list":
					reply.Items, err = out.store.List(out.ctx, out.owner, out.function, req.Collection, req.Offset)
				case "put":
					reply.Item, err = out.store.Put(out.ctx, out.owner, out.function, req.Collection, req.Key, req.Value)
				case "delete":
					err = out.store.Delete(out.ctx, out.owner, out.function, req.Collection, req.Key)
				default:
					err = ErrInvalidData
				}
			}
			if err != nil {
				if errors.Is(err, ErrInvalidData) || errors.Is(err, ErrDataNotFound) || errors.Is(err, ErrDataLimit) || errors.Is(err, ErrInvalidDatabase) || errors.Is(err, ErrSQLLimit) || errors.Is(err, ErrNotFound) {
					reply.Error = err.Error()
				} else {
					reply.Error = "function data operation failed"
				}
			} else {
				reply.OK = true
			}
		}
		encoded, err := json.Marshal(reply)
		if err != nil {
			return err
		}
		select {
		case out.input.replies <- append(encoded, '\n'):
			return nil
		case <-out.ctx.Done():
			return out.ctx.Err()
		}
	}
	_, err := out.final.Write(append(bytes.Clone(line), '\n'))
	return err
}

func (out *dataOutput) Finish() (string, error) {
	if len(bytes.TrimSpace(out.pending)) != 0 {
		if err := out.line(bytes.TrimSpace(out.pending)); err != nil {
			return "", err
		}
	}
	if out.exceeded || out.final.exceeded {
		return "", ErrOutputLimit
	}
	return strings.TrimSpace(out.final.String()), nil
}
