package mcp

import (
	"context"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Dumhouse/homepage/internal/checks"
	"github.com/Dumhouse/homepage/internal/inventory"
)

type checkOut struct {
	OK        bool   `json:"ok"`
	State     string `json:"state,omitempty"`
	Message   string `json:"message"`
	LatencyMS int    `json:"latency_ms,omitempty"`
	SSHStatus string `json:"ssh_status,omitempty"`
}

type targetArgs struct {
	TargetEntity string `json:"target_entity" jsonschema:"host, service, app_link or ssh_access"`
	TargetID     int64  `json:"target_id"`
}

type serviceCheckArgs struct {
	ServiceID int64 `json:"service_id"`
}

type hostCheckArgs struct {
	HostID int64 `json:"host_id"`
}

type problemsOut struct {
	Problems []checks.Problem `json:"problems"`
}

func addCheckTools(s *sdk.Server, inv *inventory.Store, chk *checks.Scheduler) {
	addRead(s, "list_checks", "List checks (state, last error, last result) for one target or all.",
		func(ctx context.Context, _ *sdk.CallToolRequest, a targetArgs) (*sdk.CallToolResult, []checks.Check, error) {
			list, err := chk.Store().List(ctx, a.TargetEntity, a.TargetID)
			if err != nil {
				return nil, nil, err
			}
			return text(fmt.Sprintf("%d check(s)", len(list)), list)
		})

	addRead(s, "list_problems", "List failing and down checks, worst first.",
		func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, problemsOut, error) {
			problems, err := chk.Store().ListProblems(ctx, 50)
			if err != nil {
				return nil, problemsOut{}, err
			}
			return text(fmt.Sprintf("%d problem(s)", len(problems)), problemsOut{Problems: problems})
		})

	addWrite(s, "run_check", "Run one check now and return its result.", false,
		func(ctx context.Context, _ *sdk.CallToolRequest, a idArgs) (*sdk.CallToolResult, checkOut, error) {
			if err := needWrite(ctx); err != nil {
				return nil, checkOut{}, err
			}
			res, err := chk.RunNow(ctx, a.ID)
			if err != nil {
				return nil, checkOut{}, err
			}
			out := checkOut{OK: res.OK, Message: res.Message, LatencyMS: res.LatencyMS, SSHStatus: res.SSHStatus}
			if c, err := chk.Store().Get(ctx, a.ID); err == nil {
				out.State = c.State
			}
			return text(res.Message, out)
		})

	addWrite(s, "check_service", "Probe a service now (HTTP or TCP, depending on its auto check).", false,
		func(ctx context.Context, _ *sdk.CallToolRequest, a serviceCheckArgs) (*sdk.CallToolResult, checkOut, error) {
			if err := needWrite(ctx); err != nil {
				return nil, checkOut{}, err
			}
			res, err := chk.RunTarget(ctx, "service", a.ServiceID)
			if err != nil {
				return nil, checkOut{}, err
			}
			out := checkOut{OK: res.OK, Message: res.Message, LatencyMS: res.LatencyMS}
			if list, err := chk.Store().List(ctx, "service", a.ServiceID); err == nil && len(list) > 0 {
				out.State = list[0].State
			}
			return text(res.Message, out)
		})

	addWrite(s, "check_ssh", "Probe a host's SSH login: reports key_only, password_allowed or unreachable and updates the host badge.", false,
		func(ctx context.Context, _ *sdk.CallToolRequest, a hostCheckArgs) (*sdk.CallToolResult, checkOut, error) {
			if err := needWrite(ctx); err != nil {
				return nil, checkOut{}, err
			}
			res, err := chk.CheckSSH(ctx, a.HostID)
			if err != nil {
				return nil, checkOut{}, err
			}
			return text(res.Message, checkOut{OK: res.OK, Message: res.Message, LatencyMS: res.LatencyMS, SSHStatus: res.SSHStatus})
		})
}
