/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
import { CubismIdHandle } from '../id/cubismid';
import { CubismModel } from '../model/cubismmodel';
import { CubismMotionQueueManager } from './cubismmotionqueuemanager';
/**
 * @brief パラメータに適用する表情の値を持たせる構造体
 */
export declare class ExpressionParameterValue {
    parameterId: CubismIdHandle;
    additiveValue: number;
    multiplyValue: number;
    overwriteValue: number;
}
/**
 * @brief 表情モーションの管理
 *
 * 表情モーションの管理をおこなうクラス。
 */
export declare class CubismExpressionMotionManager extends CubismMotionQueueManager {
    /**
     * コンストラクタ
     */
    constructor();
    /**
     * デストラクタ相当の処理
     */
    release(): void;
    /**
     * @brief 再生中のモーションのウェイトを取得する。
     *
     * @param[in]    index    表情のインデックス
     * @return               表情モーションのウェイト
     */
    getFadeWeight(index: number): number;
    /**
     * @brief モーションのウェイトの設定。
     *
     * @param[in]    index    表情のインデックス
     * @param[in]    index    表情モーションのウェイト
     */
    setFadeWeight(index: number, expressionFadeWeight: number): void;
    /**
     * @brief モーションの更新
     *
     * モーションを更新して、モデルにパラメータ値を反映する。
     *
     * @param[in]   model   対象のモデル
     * @param[in]   deltaTimeSeconds    デルタ時間[秒]
     * @return  true    更新されている
     *          false   更新されていない
     */
    updateMotion(model: CubismModel, deltaTimeSeconds: number): boolean;
    private _expressionParameterValues;
    private _fadeWeights;
    private _startExpressionTime;
}
import * as $ from './cubismexpressionmotionmanager';
export declare namespace Live2DCubismFramework {
    const CubismExpressionMotionManager: typeof $.CubismExpressionMotionManager;
    type CubismExpressionMotionManager = $.CubismExpressionMotionManager;
}
//# sourceMappingURL=cubismexpressionmotionmanager.d.ts.map