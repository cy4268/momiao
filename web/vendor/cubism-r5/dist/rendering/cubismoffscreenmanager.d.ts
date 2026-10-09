/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
/**
 * フレームバッファなどのコンテナのクラス
 */
declare class CubismRenderTargetContainer {
    /**
     * Constructor
     *
     * @param colorBuffer カラーバッファ
     * @param renderTexture レンダーテクスチャ
     * @param inUse 使用中かどうか
     */
    constructor(colorBuffer?: WebGLTexture, renderTexture?: WebGLFramebuffer, inUse?: boolean);
    clear(): void;
    /**
     * カラーバッファを取得
     *
     * @returns カラーバッファ
     */
    getColorBuffer(): WebGLTexture;
    /**
     * レンダーテクスチャを取得
     *
     * @returns レンダーテクスチャ
     */
    getRenderTexture(): WebGLFramebuffer;
    colorBuffer: WebGLTexture;
    renderTexture: WebGLFramebuffer;
    inUse: boolean;
}
/**
 * WebGL用オフスクリーン描画機能を管理するマネージャ
 * オフスクリーン描画機能に必要なフレームバッファなどを含むコンテナを管理する。
 * 複数のWebGLContextに対応。
 */
export declare class CubismWebGLOffscreenManager {
    /**
     * コンストラクタ
     */
    private constructor();
    /**
     * デストラクタ相当の処理
     */
    release(): void;
    /**
     * インスタンスの取得
     *
     * @return インスタンス
     */
    static getInstance(): CubismWebGLOffscreenManager;
    /**
     * WebGLContextに対応するマネージャーを取得または作成
     *
     * @param gl WebGLRenderingContextまたはWebGL2RenderingContext
     * @return WebGLContextManager
     */
    private getContextManager;
    /**
     * 指定されたWebGLContextのマネージャーを削除
     *
     * @param gl WebGLRenderingContextまたはWebGL2RenderingContext
     */
    removeContext(gl: WebGLRenderingContext | WebGL2RenderingContext): void;
    /**
     * 初期化処理
     *
     * @param gl WebGLRenderingContextまたはWebGL2RenderingContext
     * @param width 幅
     * @param height 高さ
     */
    initialize(gl: WebGLRenderingContext | WebGL2RenderingContext, width: number, height: number): void;
    /**
     * モデルを描画する前に呼び出すフレーム開始時の処理を行う
     *
     * @param gl WebGLRenderingContextまたはWebGL2RenderingContext
     */
    beginFrameProcess(gl: WebGLRenderingContext | WebGL2RenderingContext): void;
    /**
     * モデルの描画が終わった後に呼び出すフレーム終了時の処理
     *
     * @param gl WebGLRenderingContextまたはWebGL2RenderingContext
     */
    endFrameProcess(gl: WebGLRenderingContext | WebGL2RenderingContext): void;
    /**
     * コンテナサイズの取得
     *
     * @param gl WebGLRenderingContextまたはWebGL2RenderingContext
     */
    getContainerSize(gl: WebGLRenderingContext | WebGL2RenderingContext): number;
    /**
     * 使用可能なリソースコンテナの取得
     *
     * @param gl WebGLRenderingContextまたはWebGL2RenderingContext
     * @param width 幅
     * @param height 高さ
     * @param previousFramebuffer 前のフレームバッファ
     * @return 使用可能なリソースコンテナ
     */
    getOffscreenRenderTargetContainers(gl: WebGLRenderingContext | WebGL2RenderingContext, width: number, height: number, previousFramebuffer: WebGLFramebuffer): CubismRenderTargetContainer;
    /**
     * リソースコンテナの使用状態を取得
     *
     * @param gl WebGLRenderingContextまたはWebGL2RenderingContext
     * @param renderTexture WebGLFramebuffer
     * @return 使用中はtrue、未使用の場合はfalse
     */
    getUsingRenderTextureState(gl: WebGLRenderingContext | WebGL2RenderingContext, renderTexture: WebGLFramebuffer): boolean;
    /**
     * リソースコンテナの使用を開始する。
     *
     * @param gl WebGLRenderingContextまたはWebGL2RenderingContext
     * @param renderTexture WebGLFramebuffer
     */
    startUsingRenderTexture(gl: WebGLRenderingContext | WebGL2RenderingContext, renderTexture: WebGLFramebuffer): void;
    /**
     * リソースコンテナの使用を終了する。
     *
     * @param gl WebGLRenderingContextまたはWebGL2RenderingContext
     * @param renderTexture WebGLFramebuffer
     */
    stopUsingRenderTexture(gl: WebGLRenderingContext | WebGL2RenderingContext, renderTexture: WebGLFramebuffer): void;
    /**
     * リソースコンテナの使用を全て終了する。
     *
     * @param gl WebGLRenderingContextまたはWebGL2RenderingContext
     */
    stopUsingAllRenderTextures(gl: WebGLRenderingContext | WebGL2RenderingContext): void;
    /**
     * 使用されていないリソースコンテナを解放する。
     *
     * @param gl WebGLRenderingContextまたはWebGL2RenderingContext
     */
    releaseStaleRenderTextures(gl: WebGLRenderingContext | WebGL2RenderingContext): void;
    /**
     * 直前のアクティブなレンダーターゲットの最大数を取得
     *
     * @param gl WebGLRenderingContextまたはWebGL2RenderingContext
     * @returns 直前のアクティブなレンダーターゲットの最大数
     */
    getPreviousActiveRenderTextureCount(gl: WebGLRenderingContext | WebGL2RenderingContext): number;
    /**
     * 現在のアクティブなレンダーターゲットの数を取得
     *
     * @param gl WebGLRenderingContextまたはWebGL2RenderingContext
     * @returns 現在のアクティブなレンダーターゲットの数
     */
    getCurrentActiveRenderTextureCount(gl: WebGLRenderingContext | WebGL2RenderingContext): number;
    /**
     * 現在のアクティブなレンダーターゲットの数を更新
     *
     * @param gl WebGLRenderingContextまたはWebGL2RenderingContext
     */
    updateRenderTargetContainerCount(gl: WebGLRenderingContext | WebGL2RenderingContext): void;
    /**
     * 使用されていないリソースコンテナの取得
     *
     * @param gl WebGLRenderingContextまたはWebGL2RenderingContext
     * @return 使用されていないリソースコンテナ
     */
    getUnusedOffscreenRenderTargetContainer(gl: WebGLRenderingContext | WebGL2RenderingContext): CubismRenderTargetContainer;
    /**
     * 新たにリソースコンテナを作成する。
     *
     * @param gl WebGLRenderingContextまたはWebGL2RenderingContext
     * @param width 幅
     * @param height 高さ
     * @param previousFramebuffer 前のフレームバッファ
     * @return 作成されたリソースコンテナ
     */
    createOffscreenRenderTargetContainer(gl: WebGLRenderingContext | WebGL2RenderingContext, width: number, height: number, previousFramebuffer: WebGLFramebuffer): CubismRenderTargetContainer;
    private static _instance;
    private _contextManagers;
}
export {};
//# sourceMappingURL=cubismoffscreenmanager.d.ts.map