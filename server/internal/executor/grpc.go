package executor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"scheduler-platform/internal/model"

	"github.com/golang/protobuf/jsonpb"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/metadata"
)

// dynCodec 为 dynamic.Message 提供 grpc encoding.Codec，使标准 grpc 客户端
// 能编解码 dynamic.Message（其未实现新的 protoreflect.ProtoMessage 接口）。
type dynCodec struct{}

var _ encoding.Codec = dynCodec{}

func (dynCodec) Marshal(v interface{}) ([]byte, error) {
	m, ok := v.(*dynamic.Message)
	if !ok {
		return nil, fmt.Errorf("dynCodec: 不支持的类型 %T，需为 *dynamic.Message", v)
	}
	return m.Marshal()
}

func (dynCodec) Unmarshal(data []byte, v interface{}) error {
	m, ok := v.(*dynamic.Message)
	if !ok {
		return fmt.Errorf("dynCodec: 不支持的类型 %T，需为 *dynamic.Message", v)
	}
	return m.Unmarshal(data)
}

func (dynCodec) Name() string {
	return "scheduler-dynproto"
}

// withMetadata 将 map 转换为 gRPC metadata 并注入 context。
func withMetadata(ctx context.Context, kv map[string]string) context.Context {
	md := make(metadata.MD, len(kv))
	for k, v := range kv {
		md.Set(k, v)
	}
	return metadata.NewOutgoingContext(ctx, md)
}

// runGRPC 执行 gRPC 泛化调用：对任务绑定的每个节点（目标机器）都发起调用，
// target = node.address（host:port），服务名/方法名来自配置。任一节点失败则本次执行失败。
// 配置要求：service（如 hello.HelloService）、methodName（如 SayHello）、body（JSON）。
func (e *Executor) runGRPC(ctx context.Context, cfg *Config, nodes []*model.Node) (string, string, int64, []string) {
	if len(nodes) == 0 {
		return "failed", "gRPC 任务未绑定目标节点", 0,
			[]string{"[error] gRPC 任务未绑定目标节点，无法发起调用"}
	}
	if cfg.Service == "" {
		return "failed", "未配置 gRPC 服务名 service", 0,
			[]string{"[error] grpc 任务未配置 service（如 hello.HelloService）"}
	}
	if cfg.MethodName == "" {
		return "failed", "未配置 gRPC 方法名 methodName", 0,
			[]string{"[error] grpc 任务未配置 methodName（如 SayHello）"}
	}

	var logs []string
	var total int64
	allOK := true
	lastMsg := ""
	for _, n := range nodes {
		target := strings.TrimPrefix(n.Address, "grpc://")
		target = strings.TrimPrefix(target, "grpcs://")
		logs = append(logs, fmt.Sprintf("=== 节点 %s (%s) ===", n.Name, n.Address))
		st, msg, dur, ls := e.runGRPCOnce(ctx, cfg, target)
		logs = append(logs, ls...)
		total += dur
		if st != "success" {
			allOK = false
			lastMsg = msg
		}
	}
	if allOK {
		return "success", "", total, logs
	}
	return "failed", lastMsg, total, logs
}

// runGRPCOnce 向单个 target 发起 gRPC 泛化调用（通过服务端反射）。
func (e *Executor) runGRPCOnce(ctx context.Context, cfg *Config, target string) (string, string, int64, []string) {
	start := time.Now()
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(dynCodec{})),
	)
	if err != nil {
		duration := time.Since(start).Milliseconds()
		return "failed", "创建 gRPC 连接失败: " + err.Error(), duration,
			[]string{"[error] 创建 gRPC 连接失败: " + err.Error()}
	}
	defer conn.Close()

	refClient := grpcreflect.NewClientAuto(ctx, conn)
	defer refClient.Reset()

	svcDesc, err := refClient.ResolveService(cfg.Service)
	if err != nil {
		duration := time.Since(start).Milliseconds()
		return "failed", "解析服务失败: " + err.Error(), duration,
			[]string{"[error] 解析 gRPC 服务失败: " + err.Error()}
	}

	methodDesc := svcDesc.FindMethodByName(cfg.MethodName)
	if methodDesc == nil {
		duration := time.Since(start).Milliseconds()
		return "failed", fmt.Sprintf("服务 %s 中不存在方法 %s", cfg.Service, cfg.MethodName), duration,
			[]string{"[error] 方法不存在: " + cfg.MethodName}
	}

	// 用 JSON body 泛化构造请求消息
	reqMsg := dynamic.NewMessage(methodDesc.GetInputType())
	if strings.TrimSpace(cfg.Body) != "" {
		if err := reqMsg.UnmarshalJSON([]byte(cfg.Body)); err != nil {
			duration := time.Since(start).Milliseconds()
			return "failed", "解析请求 JSON 失败: " + err.Error(), duration,
				[]string{"[error] 解析请求 JSON 失败: " + err.Error()}
		}
	}

	respMsg := dynamic.NewMessage(methodDesc.GetOutputType())

	// 携带 metadata（cfg.Headers 作为 gRPC metadata）
	callCtx := ctx
	if len(cfg.Headers) > 0 {
		md := make(map[string]string, len(cfg.Headers))
		for k, v := range cfg.Headers {
			md[k] = v
		}
		callCtx = withMetadata(callCtx, md)
	}

	// 通过标准 grpc Invoke 执行泛化调用（ForceCodec 使 dynamic.Message 可编解码）
	invokeErr := conn.Invoke(callCtx, methodDesc.GetFullyQualifiedName(), reqMsg, respMsg)
	duration := time.Since(start).Milliseconds()

	logs := []string{
		fmt.Sprintf("[grpc] %s -> %s.%s (%dms)", target, cfg.Service, cfg.MethodName, duration),
	}

	if invokeErr != nil {
		return "failed", invokeErr.Error(), duration, append(logs, "[error] "+invokeErr.Error())
	}

	// 响应用 JSON 序列化，便于日志展示与变量提取
	marshaler := jsonpb.Marshaler{OrigName: true, EmitDefaults: true}
	respJSON, jerr := marshaler.MarshalToString(respMsg)
	if jerr != nil {
		logs = append(logs, "[error] 序列化响应失败: "+jerr.Error())
	} else if respJSON != "" {
		logs = append(logs, fmt.Sprintf("[resp] %s", truncate(respJSON, 1000)))
	}

	// 变量提取（响应为 JSON 时）
	if len(cfg.Extract) > 0 && jerr == nil {
		logs = append(logs, e.extractLogs("grpc", []byte(respJSON), cfg.Extract)...)
	}

	return "success", "", duration, logs
}
