// Package agent 消息采集中间件
// 通过 Eino ChatModelAgent 的 AfterModelRewriteState 钩子采集每轮 ReAct 循环中的
// 完整消息序列（assistant+tool_calls → tool_result → assistant），
// 解决 OnAgentEvents 流式事件中丢失 tool_calls 结构的问题。
//
// AfterModelRewriteState 在每次 LLM 调用完成后触发，此时 state.Messages 包含
// 自上一轮以来的所有新消息（包括 tool 结果），通过追踪 lastMsgCnt 捕获增量。
package agent

import (
	"context"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type messageCollector struct {
	mu         sync.Mutex
	messages   []*schema.Message
	lastMsgCnt int
}

func (c *messageCollector) takeAll() []*schema.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	msgs := c.messages
	c.messages = nil
	c.lastMsgCnt = 0
	return msgs
}

type collectorMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	collector *messageCollector
}

func (mw *collectorMiddleware) AfterModelRewriteState(
	ctx context.Context,
	state *adk.TypedChatModelAgentState[*schema.Message],
	mc *adk.TypedModelContext[*schema.Message],
) (context.Context, *adk.TypedChatModelAgentState[*schema.Message], error) {
	mw.collector.mu.Lock()
	defer mw.collector.mu.Unlock()

	if len(state.Messages) > mw.collector.lastMsgCnt {
		for _, m := range state.Messages[mw.collector.lastMsgCnt:] {
			mw.collector.messages = append(mw.collector.messages, m)
		}
		mw.collector.lastMsgCnt = len(state.Messages)
	}
	return ctx, state, nil
}
