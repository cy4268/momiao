const assert = require('node:assert/strict');
const RequestHandler = require('/app/src/core/RequestHandler');
const FormatConverter = require('/app/src/core/FormatConverter');
const logger = { debug() {}, info() {}, warn() {}, error() {} };
const handler = Object.create(RequestHandler.prototype);
handler.logger = logger;
handler.timeouts = { FAKE_STREAM: 1000 };
handler.formatConverter = new FormatConverter(logger, { config: {} });

async function receive(body) {
    const messages = [
        { event_type: 'chunk', data: JSON.stringify(body) },
        { type: 'STREAM_END' },
    ];
    const res = {
        __proxyApiFormat: 'openai', statusCode: 200, headersSent: false,
        status(code) { this.statusCode = code; return this; },
        type() { return this; },
        send(text) { this.body = JSON.parse(text); this.headersSent = true; return this; },
    };
    await handler._sendOpenAINonStreamResponse(
        { async dequeue() { return messages.shift(); } }, res, 'gemini-3.8-flash', 'empty-response-regression',
    );
    return res;
}

(async () => {
    // One regression scenario through the real non-stream HTTP response writer.
    for (const [name, body, status, message] of [
        ['EMPTY', {}, 502, 'Google returned no candidates'],
        ['BLOCKED', { promptFeedback: { blockReason: 'SAFETY', blockReasonMessage: 'Prompt blocked' } },
            400, 'Google rejected the prompt: SAFETY (Prompt blocked)'],
        ['UPSTREAM_ERROR', { error: { code: 429, message: 'Quota exhausted' } }, 429, 'Quota exhausted'],
    ]) {
        const res = await receive(body);
        console.log(`${name} HTTP=${res.statusCode} message=${res.body.error?.message || 'none'}`);
        assert.equal(res.statusCode, status);
        assert.equal(res.body.error.message, message);
        assert.equal(res.__usageTrackingOutcome, 'error');
        assert.equal(res.body.choices, undefined);
    }
    const normal = await receive({
        candidates: [{ content: { parts: [{ text: 'OK' }, { functionCall: { name: 'lookup', args: { id: 1 } } }] }, finishReason: 'STOP' }],
        usageMetadata: { promptTokenCount: 6, candidatesTokenCount: 2, totalTokenCount: 8 },
    });
    assert.equal(normal.statusCode, 200);
    assert.equal(normal.body.choices[0].message.content, 'OK');
    assert.equal(normal.body.choices[0].message.tool_calls[0].function.name, 'lookup');
    assert.equal(normal.body.usage.total_tokens, 8);
    console.log('NORMAL HTTP=200 text=OK tool=lookup tokens=8');
    console.log('PASS: non-stream failure propagation; normal text/tool usage preserved');
})().catch(error => { console.error(error.message); process.exitCode = 1; });
