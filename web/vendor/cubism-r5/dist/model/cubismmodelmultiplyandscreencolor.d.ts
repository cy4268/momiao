/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
import { CubismTextureColor } from '../rendering/cubismrenderer';
/**
 * SDK側から与えられた描画オブジェクトの乗算色・スクリーン色上書きフラグと
 * その色を保持する構造体
 */
export declare class ColorData {
    constructor(isOverridden?: boolean, color?: CubismTextureColor);
    isOverridden: boolean;
    color: CubismTextureColor;
}
/**
 * Handling multiply and screen colors of the model.
 */
export declare class CubismModelMultiplyAndScreenColor {
    private _model;
    private _isOverriddenModelMultiplyColors;
    private _isOverriddenModelScreenColors;
    private _userPartScreenColors;
    private _userPartMultiplyColors;
    private _userDrawableScreenColors;
    private _userDrawableMultiplyColors;
    private _userOffscreenScreenColors;
    private _userOffscreenMultiplyColors;
    /**
     * Constructor.
     *
     * @param model cubism model.
     */
    constructor(model: any);
    /**
     * Initialization for using multiply and screen colors.
     *
     * @param partCount number of parts.
     * @param drawableCount number of drawables.
     * @param offscreenCount number of offscreen.
     */
    initialize(partCount: number, drawableCount: number, offscreenCount: number): void;
    /**
     * Outputs a warning message for index out of range errors.
     *
     * @param functionName Name of the calling function
     * @param index The invalid index value
     * @param maxIndex The maximum valid index (length - 1)
     */
    private warnIndexOutOfRange;
    /**
     * Validates if the given part index is within valid range.
     *
     * @param index Part index to validate
     * @param functionName Name of the calling function for error reporting
     * @return true if the index is valid; otherwise false
     */
    private isValidPartIndex;
    /**
     * Validates if the given drawable index is within valid range.
     *
     * @param index Drawable index to validate
     * @param functionName Name of the calling function for error reporting
     * @return true if the index is valid; otherwise false
     */
    private isValidDrawableIndex;
    /**
     * Validates if the given offscreen index is within valid range.
     *
     * @param index Offscreen index to validate
     * @param functionName Name of the calling function for error reporting
     * @return true if the index is valid; otherwise false
     */
    private isValidOffscreenIndex;
    /**
     * Sets the flag indicating whether the color set at runtime is used as the multiply color for the entire model during rendering.
     *
     * @param value true if the color set at runtime is to be used; otherwise false.
     */
    setMultiplyColorEnabled(value: boolean): void;
    /**
     * Returns the flag indicating whether the color set at runtime is used as the multiply color for the entire model during rendering.
     *
     * @return true if the color set at runtime is used; otherwise false.
     */
    getMultiplyColorEnabled(): boolean;
    /**
     * Sets the flag indicating whether the color set at runtime is used as the screen color for the entire model during rendering.
     *
     * @param value true if the color set at runtime is to be used; otherwise false.
     */
    setScreenColorEnabled(value: boolean): void;
    /**
     * Returns the flag indicating whether the color set at runtime is used as the screen color for the entire model during rendering.
     *
     * @return true if the color set at runtime is used; otherwise false.
     */
    getScreenColorEnabled(): boolean;
    /**
     * Sets whether the part multiply color is overridden by the SDK.
     * Use true to use the color information from the SDK, or false to use the color information from the model.
     *
     * @param partIndex Part index
     * @param value true enable override, false to disable
     */
    setPartMultiplyColorEnabled(partIndex: number, value: boolean): void;
    /**
     * Checks whether the part multiply color is overridden by the SDK.
     *
     * @param partIndex Part index
     *
     * @return true if the color information from the SDK is used; otherwise false.
     */
    getPartMultiplyColorEnabled(partIndex: number): boolean;
    /**
     * Sets whether the part screen color is overridden by the SDK.
     * Use true to use the color information from the SDK, or false to use the color information from the model.
     *
     * @param partIndex Part index
     * @param value true enable override, false to disable
     */
    setPartScreenColorEnabled(partIndex: number, value: boolean): void;
    /**
     * Checks whether the part screen color is overridden by the SDK.
     *
     * @param partIndex Part index
     *
     * @return true if the color information from the SDK is used; otherwise false.
     */
    getPartScreenColorEnabled(partIndex: number): boolean;
    /**
     * Sets the multiply color of the part.
     *
     * @param partIndex Part index
     * @param color Multiply color to be set (CubismTextureColor)
     */
    setPartMultiplyColorByTextureColor(partIndex: number, color: CubismTextureColor): void;
    /**
     * Sets the multiply color of the part.
     *
     * @param partIndex Part index
     * @param r Red value of the multiply color to be set
     * @param g Green value of the multiply color to be set
     * @param b Blue value of the multiply color to be set
     * @param a Alpha value of the multiply color to be set
     */
    setPartMultiplyColorByRGBA(partIndex: number, r: number, g: number, b: number, a?: number): void;
    /**
     * Returns the multiply color of the part.
     *
     * @param partIndex Part index
     *
     * @return Multiply color (CubismTextureColor)
     */
    getPartMultiplyColor(partIndex: number): CubismTextureColor;
    /**
     * Sets the screen color of the part.
     *
     * @param partIndex Part index
     * @param color Screen color to be set (CubismTextureColor)
     */
    setPartScreenColorByTextureColor(partIndex: number, color: CubismTextureColor): void;
    /**
     * Sets the screen color of the part.
     *
     * @param partIndex Part index
     * @param r Red value of the screen color to be set
     * @param g Green value of the screen color to be set
     * @param b Blue value of the screen color to be set
     * @param a Alpha value of the screen color to be set
     */
    setPartScreenColorByRGBA(partIndex: number, r: number, g: number, b: number, a?: number): void;
    /**
     * Returns the screen color of the part.
     *
     * @param partIndex Part index
     *
     * @return Screen color (CubismTextureColor)
     */
    getPartScreenColor(partIndex: number): CubismTextureColor;
    /**
     * Sets the flag indicating whether the color set at runtime is used as the multiply color for the drawable during rendering.
     *
     * @param drawableIndex Drawable index
     * @param value true if the color set at runtime is to be used; otherwise false.
     */
    setDrawableMultiplyColorEnabled(drawableIndex: number, value: boolean): void;
    /**
     * Returns the flag indicating whether the color set at runtime is used as the multiply color for the drawable during rendering.
     *
     * @param drawableIndex Drawable index
     *
     * @return true if the color set at runtime is used; otherwise false.
     */
    getDrawableMultiplyColorEnabled(drawableIndex: number): boolean;
    /**
     * Sets the flag indicating whether the color set at runtime is used as the screen color for the drawable during rendering.
     *
     * @param drawableIndex Drawable index
     * @param value true if the color set at runtime is to be used; otherwise false.
     */
    setDrawableScreenColorEnabled(drawableIndex: number, value: boolean): void;
    /**
     * Returns the flag indicating whether the color set at runtime is used as the screen color for the drawable during rendering.
     *
     * @param drawableIndex Drawable index
     *
     * @return true if the color set at runtime is used; otherwise false.
     */
    getDrawableScreenColorEnabled(drawableIndex: number): boolean;
    /**
     * Sets the multiply color of the drawable.
     *
     * @param drawableIndex Drawable index
     * @param color Multiply color to be set (CubismTextureColor)
     */
    setDrawableMultiplyColorByTextureColor(drawableIndex: number, color: CubismTextureColor): void;
    /**
     * Sets the multiply color of the drawable.
     *
     * @param drawableIndex Drawable index
     * @param r Red value of the multiply color to be set
     * @param g Green value of the multiply color to be set
     * @param b Blue value of the multiply color to be set
     * @param a Alpha value of the multiply color to be set
     */
    setDrawableMultiplyColorByRGBA(drawableIndex: number, r: number, g: number, b: number, a?: number): void;
    /**
     * Returns the multiply color from the list of drawables.
     *
     * @param drawableIndex Drawable index
     *
     * @return Multiply color (CubismTextureColor)
     */
    getDrawableMultiplyColor(drawableIndex: number): CubismTextureColor;
    /**
     * Sets the screen color of the drawable.
     *
     * @param drawableIndex Drawable index
     * @param color Screen color to be set (CubismTextureColor)
     */
    setDrawableScreenColorByTextureColor(drawableIndex: number, color: CubismTextureColor): void;
    /**
     * Sets the screen color of the drawable.
     *
     * @param drawableIndex Drawable index
     * @param r Red value of the screen color to be set
     * @param g Green value of the screen color to be set
     * @param b Blue value of the screen color to be set
     * @param a Alpha value of the screen color to be set
     */
    setDrawableScreenColorByRGBA(drawableIndex: number, r: number, g: number, b: number, a?: number): void;
    /**
     * Returns the screen color from the list of drawables.
     *
     * @param drawableIndex Drawable index
     *
     * @return Screen color (CubismTextureColor)
     */
    getDrawableScreenColor(drawableIndex: number): CubismTextureColor;
    /**
     * Sets whether the offscreen multiply color is overridden by the SDK.
     * Use true to use the color information from the SDK, or false to use the color information from the model.
     *
     * @param offscreenIndex Offscreen index
     * @param value true enable override, false to disable
     */
    setOffscreenMultiplyColorEnabled(offscreenIndex: number, value: boolean): void;
    /**
     * Checks whether the offscreen multiply color is overridden by the SDK.
     *
     * @param offscreenIndex Offscreen index
     *
     * @return true if the color information from the SDK is used; otherwise false.
     */
    getOffscreenMultiplyColorEnabled(offscreenIndex: number): boolean;
    /**
     * Sets whether the offscreen screen color is overridden by the SDK.
     * Use true to use the color information from the SDK, or false to use the color information from the model.
     *
     * @param offscreenIndex Offscreen index
     * @param value true enable override, false to disable
     */
    setOffscreenScreenColorEnabled(offscreenIndex: number, value: boolean): void;
    /**
     * Checks whether the offscreen screen color is overridden by the SDK.
     *
     * @param offscreenIndex Offscreen index
     *
     * @return true if the color information from the SDK is used; otherwise false.
     */
    getOffscreenScreenColorEnabled(offscreenIndex: number): boolean;
    /**
     * Sets the multiply color of the offscreen.
     *
     * @param offscreenIndex Offsscreen index
     * @param color Multiply color to be set (CubismTextureColor)
     */
    setOffscreenMultiplyColorByTextureColor(offscreenIndex: number, color: CubismTextureColor): void;
    /**
     * Sets the multiply color of the offscreen.
     *
     * @param offscreenIndex Offsscreen index
     * @param r Red value of the multiply color to be set
     * @param g Green value of the multiply color to be set
     * @param b Blue value of the multiply color to be set
     * @param a Alpha value of the multiply color to be set
     */
    setOffscreenMultiplyColorByRGBA(offscreenIndex: number, r: number, g: number, b: number, a?: number): void;
    /**
     * Returns the multiply color from the list of offscreen.
     *
     * @param offscreenIndex Offsscreen index
     *
     * @return Multiply color (CubismTextureColor)
     */
    getOffscreenMultiplyColor(offscreenIndex: number): CubismTextureColor;
    /**
     * Sets the screen color of the offscreen.
     *
     * @param offscreenIndex Offsscreen index
     * @param color Screen color to be set (CubismTextureColor)
     */
    setOffscreenScreenColorByTextureColor(offscreenIndex: number, color: CubismTextureColor): void;
    /**
     * Sets the screen color of the offscreen.
     *
     * @param offscreenIndex Offsscreen index
     * @param r Red value of the screen color to be set
     * @param g Green value of the screen color to be set
     * @param b Blue value of the screen color to be set
     * @param a Alpha value of the screen color to be set
     */
    setOffscreenScreenColorByRGBA(offscreenIndex: number, r: number, g: number, b: number, a?: number): void;
    /**
     * Returns the screen color from the list of offscreen.
     *
     * @param offscreenIndex Offsscreen index
     *
     * @return Screen color (CubismTextureColor)
     */
    getOffscreenScreenColor(offscreenIndex: number): CubismTextureColor;
    /**
     * Sets the part color with hierarchical propagation (internal method)
     */
    private setPartColor;
    /**
     * Sets the part color enabled flag with hierarchical propagation (internal method)
     */
    private setPartColorEnabled;
}
//# sourceMappingURL=cubismmodelmultiplyandscreencolor.d.ts.map