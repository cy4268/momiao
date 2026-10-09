/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
import { CubismRenderTarget_WebGL } from './cubismrendertarget_webgl';
/**
 * WebGL用オフスクリーンサーフェス
 * マスクの描画及びオフスクリーン機能に必要なフレームバッファなどを管理する。
 */
export declare class CubismOffscreenRenderTarget_WebGL extends CubismRenderTarget_WebGL {
    /**
     * リソースコンテナマネージャを初期化する。
     *
     * @param displayBufferWidth レンダーターゲットの幅
     * @param displayBufferHeight レンダーターゲットの高さ
     */
    private initializeOffscreenManager;
    /**
     * オフスクリーン描画用レンダーターゲットをセットする。
     *
     * @param gl WebGLRenderingContextまたはWebGL2RenderingContext
     *          NOTE: Cubism 5.3以降のモデルが使用される場合はWebGL2RenderingContextを使用すること。
     * @param displayBufferWidth レンダーターゲットの幅
     * @param displayBufferHeight レンダーターゲットの高さ
     * @param previousFramebuffer 前のフレームバッファ
     */
    setOffscreenRenderTarget(gl: WebGLRenderingContext | WebGL2RenderingContext, displayBufferWidth: number, displayBufferHeight: number, previousFramebuffer: WebGLFramebuffer): void;
    /**
     * リソースコンテナの使用状態を取得
     *
     * @return 使用中はtrue、未使用の場合はfalse
     */
    getUsingRenderTextureState(): boolean;
    /**
     * リソースコンテナの使用を開始する。
     */
    startUsingRenderTexture(): void;
    /**
     * リソースコンテナの使用を終了する。
     */
    stopUsingRenderTexture(): void;
    /**
     * オフスクリーンのインデックスを設定する。
     *
     * @param offscreenIndex オフスクリーンのインデックス
     */
    setOffscreenIndex(offscreenIndex: number): void;
    /**
     * オフスクリーンのインデックスを取得する。
     *
     * @return オフスクリーンのインデックス
     */
    getOffscreenIndex(): number;
    /**
     * 以前のオフスクリーン描画用レンダーターゲットを設定する。
     *
     * @param oldOffscreen 以前のオフスクリーン描画用レンダーターゲット
     */
    setOldOffscreen(oldOffscreen: CubismOffscreenRenderTarget_WebGL): void;
    /**
     * 以前のオフスクリーン描画用レンダーターゲットを取得する。
     *
     * @return 以前のオフスクリーン描画用レンダーターゲット
     */
    getOldOffscreen(): CubismOffscreenRenderTarget_WebGL;
    /**
     * 親のオフスクリーン描画用レンダーターゲットを設定する。
     *
     * @param parentOffscreenRenderTarget 親のオフスクリーン描画用レンダーターゲット
     */
    setParentPartOffscreen(parentOffscreenRenderTarget: CubismOffscreenRenderTarget_WebGL): void;
    /**
     * 親のオフスクリーン描画用レンダーターゲットを取得する。
     *
     * @return 親のオフスクリーン描画用レンダーターゲット
     */
    getParentPartOffscreen(): CubismOffscreenRenderTarget_WebGL;
    /**
     * コンストラクタ
     */
    constructor();
    release(): void;
    private _offscreenIndex;
    private _parentOffscreenRenderTarget;
    private _oldOffscreen;
    private _webGLOffscreenManager;
    protected _gl: WebGLRenderingContext | WebGL2RenderingContext;
}
//# sourceMappingURL=cubismoffscreenrendertarget_webgl.d.ts.map