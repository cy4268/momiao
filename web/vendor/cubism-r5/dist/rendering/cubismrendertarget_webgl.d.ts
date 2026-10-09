/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
/**
 * WebGL用オフスクリーンサーフェス
 * マスクの描画に必要なフレームバッファなどを管理する。
 */
export declare class CubismRenderTarget_WebGL {
    /**
     * WebGL2RenderingContext.blitFramebuffer() でバッファのコピーを行う。
     *
     * @param src コピー元のオフスクリーンサーフェス
     * @param dst コピー先のオフスクリーンサーフェス
     */
    static copyBuffer(gl: WebGL2RenderingContext, src: CubismRenderTarget_WebGL, dst: CubismRenderTarget_WebGL): void;
    /**
     * 描画を開始する。
     *
     * @param restoreFbo EndDraw時に復元するFBOを指定する。nullを指定すると、beginDraw時に現在のFBOを記憶しておく。
     */
    beginDraw(restoreFbo?: WebGLFramebuffer): void;
    /**
     * 描画を終了し、バックバッファのサーフェイスを復元する。
     */
    endDraw(): void;
    /**
     * バインドされているカラーバッファのクリアを行う。
     *
     * @param r 赤の成分 (0.0 - 1.0)
     * @param g 緑の成分 (0.0 - 1.0)
     * @param b 青の成分 (0.0 - 1.0)
     * @param a アルファの成分 (0.0 - 1.0)
     */
    clear(r: number, g: number, b: number, a: number): void;
    /**
     * オフスクリーンサーフェスを作成する。
     *
     * @param gl WebGLRenderingContextまたはWebGL2RenderingContext
     *          NOTE: Cubism 5.3以降のモデルが使用される場合はWebGL2RenderingContextを使用すること。
     * @param displayBufferWidth オフスクリーンサーフェスの幅
     * @param displayBufferHeight オフスクリーンサーフェスの高さ
     * @param previousFramebuffer 前のフレームバッファ
     *
     * @return 成功した場合はtrue、失敗した場合はfalse
     */
    createRenderTarget(gl: WebGLRenderingContext | WebGL2RenderingContext, displayBufferWidth: number, displayBufferHeight: number, previousFramebuffer: WebGLFramebuffer): boolean;
    /**
     * レンダーターゲットを破棄する。
     */
    destroyRenderTarget(): void;
    /**
     * WebGLのコンテキストを取得する。
     *
     * @return WebGLRenderingContextまたはWebGL2RenderingContext
     */
    getGL(): WebGLRenderingContext | WebGL2RenderingContext;
    /**
     * レンダーテクスチャを取得する。
     *
     * @return WebGLFramebuffer
     */
    getRenderTexture(): WebGLFramebuffer;
    /**
     * カラーバッファを取得する。
     *
     * @return WebGLTexture
     */
    getColorBuffer(): WebGLTexture;
    /**
     * カラーバッファの幅を取得する。
     *
     * @return カラーバッファの幅
     */
    getBufferWidth(): number;
    /**
     * カラーバッファの高さを取得する。
     *
     * @return カラーバッファの高さ
     */
    getBufferHeight(): number;
    /**
     * オフスクリーンサーフェスが有効かどうかを確認する。
     *
     * @return 有効な場合はtrue、無効な場合はfalse
     */
    isValid(): boolean;
    /**
     * 以前のフレームバッファを取得する。
     *
     * @return 以前のフレームバッファ
     */
    getOldFBO(): WebGLFramebuffer;
    /**
     * コンストラクタ
     */
    constructor();
    protected _gl: WebGLRenderingContext | WebGL2RenderingContext;
    protected _colorBuffer: WebGLTexture;
    protected _renderTexture: WebGLFramebuffer;
    protected _bufferWidth: number;
    protected _bufferHeight: number;
    private _oldFbo;
}
import * as $ from './cubismrendertarget_webgl';
export declare namespace Live2DCubismFramework {
    const CubismOffscreenSurface_WebGL: typeof CubismRenderTarget_WebGL;
    type CubismOffscreenSurface_WebGL = $.CubismRenderTarget_WebGL;
}
//# sourceMappingURL=cubismrendertarget_webgl.d.ts.map