/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
import { CubismColorBlend, CubismModel, CubismAlphaBlend } from '../model/cubismmodel';
import { CubismOffscreenRenderTarget_WebGL } from './cubismoffscreenrendertarget_webgl';
import { CubismTextureColor } from './cubismrenderer';
import { CubismRenderer_WebGL } from './cubismrenderer_webgl';
/**
 * WebGL用のシェーダープログラムを生成・破棄するクラス
 */
export declare class CubismShader_WebGL {
    /**
     * 非同期でシェーダーをパスから読み込む
     *
     * @param url シェーダーのURL
     *
     * @return シェーダーのソースコード
     */
    private loadShader;
    /**
     * ブレンドモード用のシェーダーを読み込む
     */
    private loadShaders;
    /**
     * コンストラクタ
     */
    constructor();
    /**
     * デストラクタ相当の処理
     */
    release(): void;
    /**
     * 描画用のシェーダプログラムの一連のセットアップを実行する
     *
     * @param renderer レンダラー
     * @param model 描画対象のモデル
     * @param index 描画対象のメッシュのインデックス
     */
    setupShaderProgramForDrawable(renderer: CubismRenderer_WebGL, model: Readonly<CubismModel>, index: number): void;
    /**
     * オフスクリーン用のシェーダプログラムの一連のセットアップを実行する
     *
     * @param renderer レンダラー
     * @param model 描画対象のモデル
     * @param offscreen 描画対象のオフスクリーン
     */
    setupShaderProgramForOffscreen(renderer: CubismRenderer_WebGL, model: Readonly<CubismModel>, offscreen: CubismOffscreenRenderTarget_WebGL): void;
    /**
     * マスク用のシェーダプログラムの一連のセットアップを実行する
     *
     * @param renderer レンダラー
     * @param model 描画対象のモデル
     * @param index 描画対象のメッシュのインデックス
     */
    setupShaderProgramForMask(renderer: CubismRenderer_WebGL, model: Readonly<CubismModel>, index: number): void;
    /**
     * オフスクリーンのレンダリングターゲット用のシェーダープログラムを設定する
     *
     * @param renderer レンダラー
     */
    setupShaderProgramForOffscreenRenderTarget(renderer: CubismRenderer_WebGL): void;
    /**
     * オフスクリーンのレンダリングターゲットの内容をコピーする
     *
     * @param renderer レンダラー
     * @param baseColor ベースカラー
     */
    copyTexture(renderer: CubismRenderer_WebGL, baseColor: CubismTextureColor): void;
    /**
     * シェーダープログラムを解放する
     */
    releaseShaderProgram(): void;
    /**
     * シェーダープログラムを初期化する
     *
     * @param vertShaderSrc 頂点シェーダのソース
     * @param fragShaderSrc フラグメントシェーダのソース
     */
    generateShaders(): void;
    /**
     * シェーダープログラムを登録する
     */
    registerShader(): void;
    /**
     * ブレンドモード用のシェーダープログラムを登録する
     */
    registerBlendShader(): void;
    /**
     * ブレンドモード用のシェーダープログラムを生成する
     *
     * @param colorBlendMacro カラーブレンド用のマクロ
     * @param alphaBlendMacro アルファブレンド用のマクロ
     * @param shaderSetBaseIndex _shaderSets のインデックス
     */
    private generateBlendShader;
    /**
     * シェーダプログラムをロードしてアドレスを返す
     *
     * @param vertexShaderSource    頂点シェーダのソース
     * @param fragmentShaderSource  フラグメントシェーダのソース
     *
     * @return シェーダプログラムのアドレス
     */
    loadShaderProgram(vertexShaderSource: string, fragmentShaderSource: string): WebGLProgram;
    /**
     * シェーダープログラムをコンパイルする
     *
     * @param shaderType シェーダタイプ(Vertex/Fragment)
     * @param shaderSource シェーダソースコード
     *
     * @return コンパイルされたシェーダープログラム
     */
    compileShaderSource(shaderType: GLenum, shaderSource: string): WebGLProgram;
    /**
     * WebGLレンダリングコンテキストを設定する
     *
     * @param gl WebGLレンダリングコンテキスト
     */
    setGl(gl: WebGLRenderingContext | WebGL2RenderingContext): void;
    /**
     * ブレンドモード用のシェーダーパスを設定する
     *
     * @param shaderPath シェーダーパス
     */
    setShaderPath(shaderPath: string): void;
    /**
     * シェーダーパスを取得する
     *
     * @return シェーダーパス
     */
    getShaderPath(): string;
    _shaderSets: Array<CubismShaderSet>;
    gl: WebGLRenderingContext | WebGL2RenderingContext;
    _colorBlendMap: Map<CubismColorBlend, string>;
    _alphaBlendMap: Map<CubismAlphaBlend, string>;
    _colorBlendValues: Array<CubismColorBlend>;
    _alphaBlendValues: Array<CubismAlphaBlend>;
    _blendShaderSetMap: Map<string, number>;
    _shaderCount: number;
    _vertShaderSrc: string;
    _vertShaderSrcMasked: string;
    _vertShaderSrcSetupMask: string;
    _fragShaderSrcSetupMask: string;
    _fragShaderSrcPremultipliedAlpha: string;
    _fragShaderSrcMaskPremultipliedAlpha: string;
    _fragShaderSrcMaskInvertedPremultipliedAlpha: string;
    _vertShaderSrcCopy: string;
    _fragShaderSrcCopy: string;
    _fragShaderSrcColorBlend: string;
    _fragShaderSrcAlphaBlend: string;
    _vertShaderSrcBlend: string;
    _fragShaderSrcBlend: string;
    _isShaderLoading: boolean;
    _isShaderLoaded: boolean;
    _defaultShaderPath: string;
    _shaderPath: string;
}
/**
 * GLContextごとにCubismShader_WebGLを確保するためのクラス
 * シングルトンなクラスであり、CubismShaderManager_WebGL.getInstanceからアクセスする。
 */
export declare class CubismShaderManager_WebGL {
    /**
     * インスタンスを取得する（シングルトン）
     *
     * @return インスタンス
     */
    static getInstance(): CubismShaderManager_WebGL;
    /**
     * インスタンスを開放する（シングルトン）
     */
    static deleteInstance(): void;
    /**
     * Privateなコンストラクタ
     */
    private constructor();
    /**
     * デストラクタ相当の処理
     */
    release(): void;
    /**
     * GLContextをキーにShaderを取得する
     *
     * @param gl glコンテキスト
     *
     * @return shaderを返す
     */
    getShader(gl: WebGLRenderingContext): CubismShader_WebGL;
    /**
     * GLContextを登録する
     *
     * @param gl glコンテキスト
     */
    setGlContext(gl: WebGLRenderingContext): void;
    /**
     * GLContextごとのShaderを保持する変数
     */
    private _shaderMap;
}
/**
 * CubismShader_WebGLのインナークラス
 */
export declare class CubismShaderSet {
    shaderProgram: WebGLProgram;
    attributePositionLocation: GLuint;
    attributeTexCoordLocation: GLuint;
    uniformMatrixLocation: WebGLUniformLocation;
    uniformClipMatrixLocation: WebGLUniformLocation;
    samplerTexture0Location: WebGLUniformLocation;
    samplerTexture1Location: WebGLUniformLocation;
    uniformBaseColorLocation: WebGLUniformLocation;
    uniformChannelFlagLocation: WebGLUniformLocation;
    uniformMultiplyColorLocation: WebGLUniformLocation;
    uniformScreenColorLocation: WebGLUniformLocation;
    samplerFrameBufferTextureLocation: WebGLUniformLocation;
    uniformInvertMaskFlagLocation: WebGLUniformLocation;
}
/**
 * シェーダーの名前を定義する列挙型
 */
export declare enum ShaderNames {
    ShaderNames_SetupMask = 0,
    ShaderNames_NormalPremultipliedAlpha = 1,
    ShaderNames_NormalMaskedPremultipliedAlpha = 2,
    ShaderNames_NomralMaskedInvertedPremultipliedAlpha = 3,
    ShaderNames_AddPremultipliedAlpha = 4,
    ShaderNames_AddMaskedPremultipliedAlpha = 5,
    ShaderNames_AddMaskedPremultipliedAlphaInverted = 6,
    ShaderNames_MultPremultipliedAlpha = 7,
    ShaderNames_MultMaskedPremultipliedAlpha = 8,
    ShaderNames_MultMaskedPremultipliedAlphaInverted = 9,
    ShaderNames_ShaderCount = 10
}
/**
 * シェーダーの種類を定義する列挙型
 */
export declare enum ShaderType {
    ShaderType_Normal = 0,
    ShaderType_Masked = 1,
    ShaderType_MaskedInverted = 2,
    ShaderType_Count = 3
}
import * as $ from './cubismshader_webgl';
export declare namespace Live2DCubismFramework {
    const CubismShaderSet: typeof $.CubismShaderSet;
    type CubismShaderSet = $.CubismShaderSet;
    const CubismShader_WebGL: typeof $.CubismShader_WebGL;
    type CubismShader_WebGL = $.CubismShader_WebGL;
    const CubismShaderManager_WebGL: typeof $.CubismShaderManager_WebGL;
    type CubismShaderManager_WebGL = $.CubismShaderManager_WebGL;
    const ShaderNames: typeof $.ShaderNames;
    type ShaderNames = $.ShaderNames;
}
//# sourceMappingURL=cubismshader_webgl.d.ts.map