/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
import { CubismIdHandle } from '../id/cubismid';
import { CubismModel } from '../model/cubismmodel';
/**
 * ターゲットによるパラメータ追従機能
 *
 * ドラッグ入力に対するパラメータ追従機能を提供する。
 */
export declare class CubismLook {
    /**
     * インスタンスの作成
     */
    static create(): CubismLook;
    /**
     * インスタンスの破棄
     * @param instance 対象のCubismDrag
     */
    static delete(instance: CubismLook): void;
    /**
     * ターゲット追従のパラメータの紐づけ
     * @param lookParameters ターゲット追従を紐づけたいパラメータのリスト
     */
    setParameters(lookParameters: Array<LookParameterData>): void;
    /**
     * ターゲット追従に紐づいているパラメータの取得
     * @return ターゲット追従に紐づいているパラメータのリスト
     */
    getParameters(): Array<LookParameterData>;
    /**
     * モデルのパラメータの更新
     * @param model 対象のモデル
     * @param dragX ターゲットのX座標
     * @param dragY ターゲットのY座標
     */
    updateParameters(model: CubismModel, dragX: number, dragY: number): void;
    /**
     * コンストラクタ
     */
    constructor();
    _lookParameters: Array<LookParameterData>;
}
/**
 * ターゲット追従のパラメータ情報
 */
export declare class LookParameterData {
    /**
     * コンストラクタ
     * @param parameterId   ターゲット追従を紐づけるパラメータID
     * @param factorX       X方向ドラッグ入力に対する係数
     * @param factorY       Y方向ドラッグ入力に対する係数
     * @param factorXY      XY積ドラッグ入力に対する係数
     */
    constructor(parameterId?: CubismIdHandle, factorX?: number, factorY?: number, factorXY?: number);
    parameterId: CubismIdHandle;
    factorX: number;
    factorY: number;
    factorXY: number;
}
import * as $ from './cubismlook';
export declare namespace Live2DCubismFramework {
    const LookParameterData: typeof $.LookParameterData;
    type LookParameterData = $.LookParameterData;
    const CubismLook: typeof $.CubismLook;
    type CubismLook = $.CubismLook;
}
//# sourceMappingURL=cubismlook.d.ts.map