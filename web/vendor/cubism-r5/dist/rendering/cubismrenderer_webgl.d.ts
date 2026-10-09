/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
import { CubismModel } from '../model/cubismmodel';
import { CubismClippingManager } from './cubismclippingmanager';
import { CubismClippingContext, CubismRenderer, DrawableObjectType } from './cubismrenderer';
/**
 * クリッピングマスクの処理を実行するクラス
 */
export declare class CubismClippingManager_WebGL extends CubismClippingManager<CubismClippingContext_WebGL> {
    /**
     * WebGLレンダリングコンテキストを設定する
     *
     * @param gl WebGLレンダリングコンテキスト
     */
    setGL(gl: WebGLRenderingContext): void;
    /**
     * コンストラクタ
     */
    constructor();
    /**
     * クリッピングコンテキストを作成する。モデル描画時に実行する。
     *
     * @param model モデルのインスタンス
     * @param renderer レンダラのインスタンス
     * @param lastFbo フレームバッファ
     * @param lastViewport ビューポート
     * @param drawObjectType 描画オブジェクトのタイプ
     */
    setupClippingContext(model: CubismModel, renderer: CubismRenderer_WebGL, lastFbo: WebGLFramebuffer, lastViewport: number[], drawObjectType: DrawableObjectType): void;
    /**
     * マスクの合計数をカウント
     *
     * @return マスクの合計数を返す
     */
    getClippingMaskCount(): number;
    _currentMaskBuffer: CubismRenderTarget_WebGL;
    gl: WebGLRenderingContext;
}
/**
 * クリッピングマスクのコンテキスト
 */
export declare class CubismClippingContext_WebGL extends CubismClippingContext {
    /**
     * 引数付きコンストラクタ
     *
     * @param manager マスクを管理しているマネージャのインスタンス
     * @param clippingDrawableIndices クリップしているDrawableのインデックスリスト
     * @param clipCount クリップしているDrawableの個数
     */
    constructor(manager: CubismClippingManager_WebGL, clippingDrawableIndices: Int32Array, clipCount: number);
    /**
     * このマスクを管理するマネージャのインスタンスを取得する
     *
     * @return クリッピングマネージャのインスタンス
     */
    getClippingManager(): CubismClippingManager_WebGL;
    /**
     * WebGLレンダリングコンテキストを設定する
     *
     * @param gl WebGLレンダリングコンテキスト
     */
    setGl(gl: WebGLRenderingContext): void;
    private _owner;
}
/**
 * Cubismモデルを描画する直前のWebGLのステートを保持・復帰させるクラス
 */
export declare class CubismRendererProfile_WebGL {
    /**
     * WebGLの有効・無効をセットする
     *
     * @param index 有効・無効にする機能
     * @param enabled trueなら有効にする
     */
    private setGlEnable;
    /**
     * WebGLのVertex Attribute Array機能の有効・無効をセットする
     *
     * @param   index   有効・無効にする機能
     * @param   enabled trueなら有効にする
     */
    private setGlEnableVertexAttribArray;
    /**
     * WebGLのステートを保持する
     */
    save(): void;
    /**
     * 保持したWebGLのステートを復帰させる
     */
    restore(): void;
    /**
     * WebGLレンダリングコンテキストを設定する
     *
     * @param gl WebGLレンダリングコンテキスト
     */
    setGl(gl: WebGLRenderingContext): void;
    /**
     * コンストラクタ
     */
    constructor();
    private _lastArrayBufferBinding;
    private _lastElementArrayBufferBinding;
    private _lastProgram;
    private _lastActiveTexture;
    private _lastTexture0Binding2D;
    private _lastTexture1Binding2D;
    private _lastVertexAttribArrayEnabled;
    private _lastScissorTest;
    private _lastBlend;
    private _lastStencilTest;
    private _lastDepthTest;
    private _lastCullFace;
    private _lastFrontFace;
    private _lastColorMask;
    private _lastBlending;
    gl: WebGLRenderingContext;
}
/**
 * WebGL用の描画命令を実装したクラス
 */
export declare class CubismRenderer_WebGL extends CubismRenderer {
    /**
     * レンダラの初期化処理を実行する
     * 引数に渡したモデルからレンダラの初期化処理に必要な情報を取り出すことができる
     * NOTE: WebGLコンテキストが初期化されていない可能性があるため、ここではWebGLコンテキストを使う初期化は行わない。
     *
     * @param model モデルのインスタンス
     * @param maskBufferCount バッファの生成数
     */
    initialize(model: CubismModel, maskBufferCount?: number): void;
    /**
     * オフスクリーンの親を探して設定する
     *
     * @param model モデルのインスタンス
     * @param offscreenCount オフスクリーンの数
     */
    private setupParentOffscreens;
    /**
     * WebGLテクスチャのバインド処理
     * CubismRendererにテクスチャを設定し、CubismRenderer内でその画像を参照するためのIndex値を戻り値とする
     *
     * @param modelTextureNo セットするモデルテクスチャの番号
     * @param glTextureNo WebGLテクスチャの番号
     */
    bindTexture(modelTextureNo: number, glTexture: WebGLTexture): void;
    /**
     * WebGLにバインドされたテクスチャのリストを取得する
     *
     * @return テクスチャのリスト
     */
    getBindedTextures(): Map<number, WebGLTexture>;
    /**
     * クリッピングマスクバッファのサイズを設定する
     * マスク用のFrameBufferを破棄、再作成する為処理コストは高い
     *
     * @param size クリッピングマスクバッファのサイズ
     */
    setClippingMaskBufferSize(size: number): void;
    /**
     * クリッピングマスクバッファのサイズを取得する
     *
     * @return クリッピングマスクバッファのサイズ
     */
    getClippingMaskBufferSize(): number;
    /**
     * ブレンドモード用のフレームバッファを取得する
     *
     * @return ブレンドモード用のフレームバッファ
     */
    getModelRenderTarget(index: number): CubismRenderTarget_WebGL;
    /**
     * レンダーテクスチャの枚数を取得する
     * @return レンダーテクスチャの枚数
     */
    getRenderTextureCount(): number;
    /**
     * コンストラクタ
     */
    constructor(width: number, height: number);
    /**
     * デストラクタ相当の処理
     */
    release(): void;
    /**
     * Shaderの読み込みを行う
     * @param shaderPath シェーダのパス
     */
    loadShaders(shaderPath?: string): void;
    /**
     * モデルを描画する実際の処理
     * @param shaderPath シェーダのパス
     */
    doDrawModel(shaderPath?: string): void;
    /**
     * 描画オブジェクトのループ処理を行う。
     *
     * @param lastFbo 前回のフレームバッファ
     */
    drawObjectLoop(lastFbo: WebGLFramebuffer): void;
    /**
     * 描画オブジェクトを描画する。
     *
     * @param objectIndex 描画対象のオブジェクトのインデックス
     * @param objectType 描画対象のオブジェクトのタイプ
     * @param lastFbo 前回のフレームバッファ
     * @param lastViewport 前回のビューポート
     */
    protected renderObject(objectIndex: number, objectType: DrawableObjectType): void;
    /**
     * 描画オブジェクト（アートメッシュ）を描画する。
     *
     * @param model 描画対象のモデル
     * @param index 描画対象のメッシュのインデックス
     */
    drawDrawable(drawableIndex: number, rootFbo: WebGLFramebuffer): void;
    /**
     * 描画オブジェクト（アートメッシュ）を描画する。
     *
     * @param model 描画対象のモデル
     * @param index 描画対象のメッシュのインデックス
     */
    drawMeshWebGL(model: Readonly<CubismModel>, index: number): void;
    /**
     * オフスクリーンを親のオフスクリーンにコピーする。
     *
     * @param objectIndex オブジェクトのインデックス
     * @param objectType  オブジェクトの種類
     */
    submitDrawToParentOffscreen(objectIndex: number, objectType: DrawableObjectType): void;
    /**
     * 描画オブジェクト（オフスクリーン）を追加する。
     *
     * @param offscreenIndex オフスクリーンのインデックス
     */
    addOffscreen(offscreenIndex: number): void;
    /**
     * オフスクリーン描画を行う。
     *
     * @param offscreen オフスクリーンレンダリングターゲット
     */
    drawOffscreen(offscreen: CubismOffscreenRenderTarget_WebGL): void;
    /**
     * オフスクリーン描画のWebGL実装
     *
     * @param model モデル
     * @param index オフスクリーンインデックス
     */
    drawOffscreenWebGL(model: Readonly<CubismModel>, offscreen: CubismOffscreenRenderTarget_WebGL): void;
    /**
     * モデル描画直前のレンダラのステートを保持する
     */
    protected saveProfile(): void;
    /**
     * モデル描画直前のレンダラのステートを復帰させる
     */
    protected restoreProfile(): void;
    /**
     * モデル描画直前のオフスクリーン設定を行う
     */
    beforeDrawModelRenderTarget(): void;
    /**
     * モデル描画後のオフスクリーン設定を行う
     */
    afterDrawModelRenderTarget(): void;
    /**
     * オフスクリーンのクリッピングマスクのバッファを取得する
     *
     * @param index オフスクリーンのクリッピングマスクのバッファのインデックス
     *
     * @return オフスクリーンのクリッピングマスクのバッファへのポインタ
     */
    getOffscreenMaskBuffer(index: number): CubismRenderTarget_WebGL;
    /**
     * レンダラが保持する静的なリソースを解放する
     * WebGLの静的なシェーダープログラムを解放する
     */
    static doStaticRelease(): void;
    /**
     * レンダーステートを設定する
     *
     * @param fbo アプリケーション側で指定しているフレームバッファ
     * @param viewport ビューポート
     */
    setRenderState(fbo: WebGLFramebuffer, viewport: number[]): void;
    /**
     * 描画開始時の追加処理
     * モデルを描画する前にクリッピングマスクに必要な処理を実装している
     */
    preDraw(): void;
    /**
     * Drawableのマスク用のオフスクリーンサーフェースを取得する
     *
     * @param index オフスクリーンサーフェースのインデックス
     *
     * @return マスク用のオフスクリーンサーフェース
     */
    getDrawableMaskBuffer(index: number): CubismRenderTarget_WebGL;
    /**
     * マスクテクスチャに描画するクリッピングコンテキストをセットする
     */
    setClippingContextBufferForMask(clip: CubismClippingContext_WebGL): void;
    /**
     * マスクテクスチャに描画するクリッピングコンテキストを取得する
     *
     * @return マスクテクスチャに描画するクリッピングコンテキスト
     */
    getClippingContextBufferForMask(): CubismClippingContext_WebGL;
    /**
     * Drawableの画面上に描画するクリッピングコンテキストをセットする
     *
     * @param clip drawableで画面上に描画するクリッピングコンテキスト
     */
    setClippingContextBufferForDrawable(clip: CubismClippingContext_WebGL): void;
    /**
     * Drawableの画面上に描画するクリッピングコンテキストを取得する
     *
     * @return Drawableの画面上に描画するクリッピングコンテキスト
     */
    getClippingContextBufferForDrawable(): CubismClippingContext_WebGL;
    /**
     * offscreenで画面上に描画するクリッピングコンテキストをセットする。
     *
     * @param clip offscreenで画面上に描画するクリッピングコンテキスト
     */
    setClippingContextBufferForOffscreen(clip: CubismClippingContext_WebGL): void;
    /**
     * offscreenで画面上に描画するクリッピングコンテキストを取得する。
     *
     * @return offscreenで画面上に描画するクリッピングコンテキスト
     */
    getClippingContextBufferForOffscreen(): CubismClippingContext_WebGL;
    /**
     * マスク生成時かを判定する
     *
     * @return 判定値
     */
    isGeneratingMask(): boolean;
    /**
     * glの設定
     */
    startUp(gl: WebGLRenderingContext | WebGL2RenderingContext): void;
    _textures: Map<number, WebGLTexture>;
    _sortedObjectsIndexList: Array<number>;
    _sortedObjectsTypeList: Array<number>;
    _rendererProfile: CubismRendererProfile_WebGL;
    _drawableClippingManager: CubismClippingManager_WebGL;
    _clippingContextBufferForMask: CubismClippingContext_WebGL;
    _clippingContextBufferForDraw: CubismClippingContext_WebGL;
    _clippingContextBufferForOffscreen: CubismClippingContext_WebGL;
    _offscreenClippingManager: CubismClippingManager_WebGL;
    _modelRenderTargets: Array<CubismOffscreenRenderTarget_WebGL>;
    _drawableMasks: Array<CubismRenderTarget_WebGL>;
    _offscreenMasks: Array<CubismRenderTarget_WebGL>;
    _offscreenList: Array<CubismOffscreenRenderTarget_WebGL>;
    _currentFbo: WebGLFramebuffer;
    _currentOffscreen: CubismOffscreenRenderTarget_WebGL | null;
    _modelRootFbo: WebGLFramebuffer;
    _bufferData: {
        vertex: WebGLBuffer;
        uv: WebGLBuffer;
        index: WebGLBuffer;
    };
    _extension: any;
    gl: WebGLRenderingContext | WebGL2RenderingContext;
}
import * as $ from './cubismrenderer_webgl';
import { CubismRenderTarget_WebGL as CubismRenderTarget_WebGL } from './cubismrendertarget_webgl';
import { CubismOffscreenRenderTarget_WebGL as CubismOffscreenRenderTarget_WebGL } from './cubismoffscreenrendertarget_webgl';
export declare namespace Live2DCubismFramework {
    const CubismClippingContext: typeof CubismClippingContext_WebGL;
    type CubismClippingContext = $.CubismClippingContext_WebGL;
    const CubismClippingManager_WebGL: typeof $.CubismClippingManager_WebGL;
    type CubismClippingManager_WebGL = $.CubismClippingManager_WebGL;
    const CubismRenderer_WebGL: typeof $.CubismRenderer_WebGL;
    type CubismRenderer_WebGL = $.CubismRenderer_WebGL;
}
//# sourceMappingURL=cubismrenderer_webgl.d.ts.map