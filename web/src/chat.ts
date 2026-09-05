import { ApiError } from './api';
export interface ChatInput { model: string; group: string; prompt: string; maxTokens: number }
export interface ChatResult { text: string; reasoning: string; usage?: { prompt_tokens?: number; completion_tokens?: number; total_tokens?: number } }
const limit = 1024 * 1024;
export const aborted = () => new ApiError('已停止接收；停止不保证上游停止计费。', 0, 'ABORTED');
export const modelFailure = (status = 0) => new ApiError(status === 401 ? '登录已失效，请重新登录。' : status === 403 ? '请求被拒绝，请检查分组权限与账户额度。' : `模型请求未完成${status ? `（HTTP ${status}）` : ''}，请检查模型或渠道配置。`, status);
// Narrow text SSE reader: a five-minute caller timeout and a 1 MiB wire budget.
export async function readChat(response: Response, signal: AbortSignal, onUpdate: (r: ChatResult) => void): Promise<ChatResult> {
    if (!response.ok) { void response.body?.cancel().catch(() => {}); throw modelFailure(response.status); }
    if (!response.body) throw modelFailure();
    const reader = response.body.getReader();
    const cancel = () => { void reader.cancel().catch(() => {}); };
    signal.addEventListener('abort', cancel, { once: true });
    let bytes = 0, buffer = '', done = false;
    const result: ChatResult = { text: '', reasoning: '' };
    const decoder = new TextDecoder('utf-8', { fatal: true });
    const isSSE = response.headers.get('content-type')?.includes('text/event-stream');
    function parse(text: string, streaming: boolean) {
        let event;
        try { event = JSON.parse(text); } catch { throw new ApiError('模型响应格式异常。', 0, 'STREAM_FORMAT'); }
        if (!event || typeof event !== 'object' || event.error || event.success === false) throw modelFailure();
        const choice = event.choices?.[0];
        const delta = streaming ? choice?.delta : choice?.message;
        if (delta?.content != null && typeof delta.content !== 'string') throw modelFailure();
        if (delta?.reasoning_content != null && typeof delta.reasoning_content !== 'string') throw modelFailure();
        result.text += delta?.content || '';
        result.reasoning += delta?.reasoning_content || '';
        if (event.usage && typeof event.usage === 'object') {
            const usage: NonNullable<ChatResult['usage']> = {};
            for (const key of ['prompt_tokens', 'completion_tokens', 'total_tokens'] as const) {
                if (Number.isSafeInteger(event.usage[key]) && event.usage[key] >= 0) usage[key] = event.usage[key];
            }
            if (Object.keys(usage).length) result.usage = usage;
        }
        if (!streaming && !choice?.message) throw modelFailure();
        if (signal.aborted) throw aborted();
        onUpdate({ ...result });
    }
    function frames() {
        let match: RegExpExecArray | null;
        while ((match = /\r?\n\r?\n/.exec(buffer))) {
            const frame = buffer.slice(0, match.index);
            buffer = buffer.slice(match.index + match[0].length);
            const data = frame.split(/\r?\n/).filter(line => line.startsWith('data:')).map(line => line.slice(5).replace(/^ /, '')).join('\n');
            if (!data.trim()) continue;
            if (data.trim() === '[DONE]') { done = true; return; }
            parse(data, true);
        }
    }
    try {
        if (signal.aborted) throw aborted();
        while (!done) {
            const chunk = await reader.read();
            if (signal.aborted) throw aborted();
            if (chunk.done) {
                buffer += decoder.decode();
                if (isSSE) { frames(); if (!done) throw new ApiError('连接提前结束，响应可能不完整；未自动重试。', 0, 'STREAM_EOF'); }
                else { parse(buffer, false); done = true; }
                break;
            }
            bytes += chunk.value.byteLength;
            if (bytes > limit) throw new ApiError('响应超过 1 MiB 限制，已停止接收；内容不完整。', 0, 'STREAM_LIMIT');
            buffer += decoder.decode(chunk.value, { stream: true });
            if (isSSE) frames();
        }
        return result;
    } finally { signal.removeEventListener('abort', cancel); await reader.cancel().catch(() => {}); reader.releaseLock(); }
}
