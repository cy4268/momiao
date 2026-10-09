import type { CubismMoc } from '../vendor/cubism-r5/dist/model/cubismmoc';
import type { CubismModel } from '../vendor/cubism-r5/dist/model/cubismmodel';
import type { CubismMotionManager } from '../vendor/cubism-r5/dist/motion/cubismmotionmanager';
import type { CubismPhysics } from '../vendor/cubism-r5/dist/physics/cubismphysics';
import type { CubismRenderer_WebGL } from '../vendor/cubism-r5/dist/rendering/cubismrenderer_webgl';

export type GuangxiRuntime = { setActive(active: boolean): void; resize(): void; dispose(): void };
const MODEL_PATH = '/assets/guangxi/v1/runtime/guangxi-idle.model3.json';
const SHADER_PATH = '/vendor/cubism-r5/Shaders/WebGL/';
let coreReady: Promise<void> | undefined;
let rendererCount = 0;

function loadCore(): Promise<void> {
    if (typeof Live2DCubismCore !== 'undefined') return Promise.resolve();
    if (!coreReady) {
        coreReady = new Promise<void>((resolve, reject) => {
            const script = document.createElement('script');
            const finish = (error?: Error) => {
                window.clearTimeout(timeout);
                script.onload = script.onerror = null;
                if (error) { script.remove(); reject(error); } else resolve();
            };
            const timeout = window.setTimeout(() => finish(new Error('Cubism Core timeout')), 15_000);
            script.src = '/vendor/cubism-r5/live2dcubismcore.min.js';
            script.async = true;
            script.onload = () => finish(typeof Live2DCubismCore === 'undefined' ? new Error('Cubism Core missing') : undefined);
            script.onerror = () => finish(new Error('Cubism Core load failed'));
            document.head.append(script);
        }).catch(error => { coreReady = undefined; throw error; });
    }
    return coreReady;
}

async function abortable<T>(promise: Promise<T>, signal: AbortSignal): Promise<T> {
    signal.throwIfAborted();
    let abort!: () => void;
    try {
        return await Promise.race([promise, new Promise<never>((_, reject) => {
            abort = () => reject(signal.reason);
            signal.addEventListener('abort', abort, { once: true });
        })]);
    } finally { signal.removeEventListener('abort', abort); }
}

async function request(path: string | URL, signal: AbortSignal): Promise<Response> {
    const url = new URL(path, location.href);
    if (url.origin !== location.origin) throw new Error('Unexpected asset origin');
    const response = await fetch(url, { signal, credentials: 'same-origin' });
    if (!response.ok) throw new Error(`Asset ${url.pathname}: HTTP ${response.status}`);
    return response;
}

async function decodeTexture(blob: Blob, signal: AbortSignal): Promise<HTMLImageElement> {
    const reader = new FileReader();
    try {
        // The portal permits data: images, not blob: URLs. Keep its CSP unchanged.
        const source = await abortable(new Promise<string>((resolve, reject) => {
            reader.onload = () => resolve(reader.result as string);
            reader.onerror = () => reject(reader.error ?? new Error('Texture read failed'));
            reader.readAsDataURL(blob);
        }), signal);
        const image = new Image();
        image.src = source;
        await abortable(image.decode(), signal);
        return image;
    } finally {
        reader.onload = reader.onerror = null;
        if (reader.readyState === FileReader.LOADING) reader.abort();
    }
}

// R5's generateShaders() is fire-and-forget and neither checks HTTP status nor accepts
// an AbortSignal. Fill its public, version-pinned source fields, then use the original
// synchronous compiler. No SDK source is patched and no fetch survives this mount.
const shaderFiles = {
    _vertShaderSrc: 'vertshadersrc.vert', _vertShaderSrcMasked: 'vertshadersrcmasked.vert',
    _vertShaderSrcSetupMask: 'vertshadersrcsetupmask.vert', _fragShaderSrcSetupMask: 'fragshadersrcsetupmask.frag',
    _fragShaderSrcPremultipliedAlpha: 'fragshadersrcpremultipliedalpha.frag',
    _fragShaderSrcMaskPremultipliedAlpha: 'fragshadersrcmaskpremultipliedalpha.frag',
    _fragShaderSrcMaskInvertedPremultipliedAlpha: 'fragshadersrcmaskinvertedpremultipliedalpha.frag',
    _vertShaderSrcCopy: 'vertshadersrccopy.vert', _fragShaderSrcCopy: 'fragshadersrccopy.frag',
    _fragShaderSrcColorBlend: 'fragshadersrccolorblend.frag', _fragShaderSrcAlphaBlend: 'fragshadersrcalphablend.frag',
    _vertShaderSrcBlend: 'vertshadersrcblend.vert', _fragShaderSrcBlend: 'fragshadersrcpremultipliedalphablend.frag',
} as const;

export async function mountGuangxi(canvas: HTMLCanvasElement, signal: AbortSignal): Promise<GuangxiRuntime> {
    const loading = new AbortController();
    const cancel = () => loading.abort(signal.reason);
    signal.addEventListener('abort', cancel, { once: true });
    if (signal.aborted) cancel();
    const pending = loading.signal;
    let moc: CubismMoc | null = null;
    let model: CubismModel | null = null;
    let renderer: CubismRenderer_WebGL | null = null;
    let physics: CubismPhysics | null = null;
    let motions: CubismMotionManager | null = null;
    let texture: WebGLTexture | null = null;
    let gl: WebGLRenderingContext | null = null;
    let registered = false;
    let releasePrograms = () => {};
    let releaseShaders = () => {};
    let disposed = false;
    let active = false;
    let frame = 0;
    let lastTime = 0;
    let animationTime = 0;
    let blinkStart = 3 + Math.random() * 3;
    const lost = () => dispose();

    function dispose() {
        if (disposed) return;
        disposed = true;
        active = false;
        loading.abort();
        signal.removeEventListener('abort', cancel);
        cancelAnimationFrame(frame);
        canvas.removeEventListener('webglcontextlost', lost);
        motions?.release();
        physics?.release();
        renderer?.release();
        if (texture) gl?.deleteTexture(texture);
        releasePrograms();
        if (registered && --rendererCount === 0) releaseShaders();
        if (model) moc?.deleteModel(model);
        moc?.release();
        // Frees the context's remaining driver resources as well as SDK allocations.
        gl?.getExtension('WEBGL_lose_context')?.loseContext();
        motions = physics = renderer = model = moc = texture = gl = null;
    }

    function resize() {
        if (disposed) return;
        const box = canvas.getBoundingClientRect();
        const dpr = Math.min(window.devicePixelRatio || 1, 2);
        const width = Math.max(1, Math.round(box.width * dpr));
        const height = Math.max(1, Math.round(box.height * dpr));
        if (canvas.width !== width || canvas.height !== height) { canvas.width = width; canvas.height = height; }
    }

    try {
        pending.throwIfAborted();
        await abortable(loadCore(), pending);
        // R5 enums read Core at module evaluation time, not only during startUp.
        const [{ CubismFramework }, { CubismMoc }, { CubismMatrix44 }, { CubismModelMatrix },
            { CubismMotion }, { CubismMotionManager }, { CubismPhysics }, { CubismRenderer_WebGL },
            { CubismShaderManager_WebGL, CubismShaderSet }] = await abortable(Promise.all([
                import('../vendor/cubism-r5/dist/live2dcubismframework'),
                import('../vendor/cubism-r5/dist/model/cubismmoc'),
                import('../vendor/cubism-r5/dist/math/cubismmatrix44'),
                import('../vendor/cubism-r5/dist/math/cubismmodelmatrix'),
                import('../vendor/cubism-r5/dist/motion/cubismmotion'),
                import('../vendor/cubism-r5/dist/motion/cubismmotionmanager'),
                import('../vendor/cubism-r5/dist/physics/cubismphysics'),
                import('../vendor/cubism-r5/dist/rendering/cubismrenderer_webgl'),
                import('../vendor/cubism-r5/dist/rendering/cubismshader_webgl'),
            ]), pending);
        releaseShaders = () => CubismShaderManager_WebGL.deleteInstance();
        pending.throwIfAborted();
        if (!CubismFramework.isStarted()) CubismFramework.startUp();
        if (!CubismFramework.isInitialized()) CubismFramework.initialize();
        const setting = await (await request(MODEL_PATH, pending)).json();
        const references = setting.FileReferences;
        const resource = (path: string) => request(new URL(path, new URL(MODEL_PATH, location.href)), pending);
        const [mocBytes, motionBytes, physicsBytes, textureBlob, sources] = await Promise.all([
            resource(references.Moc).then(response => response.arrayBuffer()),
            resource(references.Motions.Idle[0].File).then(response => response.arrayBuffer()),
            resource(references.Physics).then(response => response.arrayBuffer()),
            resource(references.Textures[0]).then(response => response.blob()),
            Promise.all(Object.entries(shaderFiles).map(async ([field, filename]) => [field, await (await request(SHADER_PATH + filename, pending)).text()] as const)),
        ]);
        const image = await decodeTexture(textureBlob, pending);
        pending.throwIfAborted();
        gl = canvas.getContext('webgl', { alpha: true, premultipliedAlpha: true, antialias: true });
        if (!gl) throw new Error('WebGL not available');
        canvas.addEventListener('webglcontextlost', lost);
        moc = CubismMoc.create(mocBytes, true);
        model = moc?.createModel() ?? null;
        if (!model) throw new Error('MOC model creation failed');
        resize();
        renderer = new CubismRenderer_WebGL(canvas.width, canvas.height);
        renderer.initialize(model);
        rendererCount++;
        registered = true;
        renderer.startUp(gl);
        const shader = CubismShaderManager_WebGL.getInstance().getShader(gl);
        releasePrograms = () => {
            // Failed compilation uses numeric zero in R5; WebGL deletion expects null instead.
            for (const set of shader._shaderSets) if (!set.shaderProgram) set.shaderProgram = null as unknown as WebGLProgram;
            shader.release();
            shader._shaderSets.length = 0; // The shared manager may later release this entry too.
        };
        shader.setShaderPath(SHADER_PATH);
        Object.assign(shader, Object.fromEntries(sources));
        shader._shaderSets = Array.from({ length: shader._shaderCount }, () => new CubismShaderSet());
        shader.registerShader();
        shader.registerBlendShader();
        // R5 reserves three unused slots for Normal/Over, which uses the compatibility shader.
        const usedPrograms = [...Array.from({ length: 11 }, (_, index) => index),
            ...Array.from(shader._blendShaderSetMap.values()).flatMap(index => [index, index + 1, index + 2])];
        const invalidPrograms = usedPrograms.filter(index => {
            const program = shader._shaderSets[index]?.shaderProgram;
            return !program || !gl!.getProgramParameter(program, gl!.LINK_STATUS);
        });
        if (invalidPrograms.length) throw new Error(`Shader compilation failed: ${invalidPrograms.join(',')} of ${shader._shaderCount}`);
        shader._isShaderLoaded = true;
        texture = gl.createTexture();
        if (!texture) throw new Error('Texture creation failed');
        gl.bindTexture(gl.TEXTURE_2D, texture);
        gl.pixelStorei(gl.UNPACK_PREMULTIPLY_ALPHA_WEBGL, 1);
        gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, image);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR_MIPMAP_LINEAR);
        gl.generateMipmap(gl.TEXTURE_2D);
        gl.bindTexture(gl.TEXTURE_2D, null);
        renderer.bindTexture(0, texture);
        renderer.setIsPremultipliedAlpha(true);
        const motion = CubismMotion.create(motionBytes, motionBytes.byteLength);
        motion.setEffectIds([], []); // Idle owns body parameters; the local clock alone owns the eye.
        motion.setLoop(true);
        motion.setLoopFadeIn(false);
        motion.setFadeInTime(0);
        motion.setFadeOutTime(0);
        motions = new CubismMotionManager();
        motions.startMotionPriority(motion, true, 1);
        // This rig has angle-only inputs. R5 also uses the otherwise unused Position
        // normalization maximum as its particle snap-to-zero threshold. At 10 the
        // subtle idle never leaves zero; 0.1 preserves the angle/output calibration.
        const webPhysics = JSON.parse(new TextDecoder().decode(physicsBytes));
        webPhysics.PhysicsSettings[0].Normalization.Position = { Minimum: -0.1, Default: 0, Maximum: 0.1 };
        const webPhysicsBytes = new TextEncoder().encode(JSON.stringify(webPhysics)).buffer;
        physics = CubismPhysics.create(webPhysicsBytes, webPhysicsBytes.byteLength);
        physics.stabilization(model);
        model.saveParameters();
        const eye = CubismFramework.getIdManager().getId('ParamEyeROpen');
        const modelMatrix = new CubismModelMatrix(model.getCanvasWidth(), model.getCanvasHeight());
        modelMatrix.setHeight(1.9);
        const projection = new CubismMatrix44();

        const draw = (delta: number) => {
            animationTime += delta;
            model!.loadParameters();
            motions!.updateMotion(model!, delta);
            model!.saveParameters();
            let open = 1;
            const blink = animationTime - blinkStart;
            if (blink >= 0.24) blinkStart += 3 + Math.random() * 3;
            else if (blink >= 0.12) open = (blink - 0.12) / 0.12;
            else if (blink >= 0.08) open = 0;
            else if (blink >= 0) open = 1 - blink / 0.08;
            model!.setParameterValueById(eye, open);
            physics!.evaluate(model!, delta);
            model!.update();
            resize();
            projection.loadIdentity();
            projection.scale(canvas.height / canvas.width, 1);
            projection.multiplyByMatrix(modelMatrix);
            renderer!.setMvpMatrix(projection);
            // R5's declaration omits null although its implementation uses the default framebuffer.
            renderer!.setRenderState(null as unknown as WebGLFramebuffer, [0, 0, canvas.width, canvas.height]);
            gl!.clearColor(0, 0, 0, 0);
            gl!.clear(gl!.COLOR_BUFFER_BIT);
            renderer!.drawModel(SHADER_PATH);
        };
        const tick = (time: number) => {
            if (!active || disposed) return;
            if (!lastTime) lastTime = time;
            const elapsed = time - lastTime;
            if (elapsed + 0.25 >= 1000 / 30) {
                try { draw(Math.min(elapsed / 1000, 0.05)); }
                catch { dispose(); canvas.dispatchEvent(new Event('error')); return; }
                lastTime = time;
            }
            frame = requestAnimationFrame(tick);
        };
        draw(0);
        if (gl.isContextLost() || gl.getError() !== gl.NO_ERROR) throw new Error('First draw failed');
        signal.removeEventListener('abort', cancel);
        return { resize, dispose, setActive(value) {
            if (disposed || active === value) return;
            active = value;
            lastTime = 0;
            if (active) frame = requestAnimationFrame(tick);
            else cancelAnimationFrame(frame);
        } };
    } catch (error) { dispose(); throw error; }
}
