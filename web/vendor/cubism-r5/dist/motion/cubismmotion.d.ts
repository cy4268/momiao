/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
import { CubismIdHandle } from '../id/cubismid';
import { CubismModel } from '../model/cubismmodel';
import { ACubismMotion, BeganMotionCallback, FinishedMotionCallback } from './acubismmotion';
import { CubismMotionData } from './cubismmotioninternal';
import { CubismMotionQueueEntry } from './cubismmotionqueueentry';
/**
 * Enumerator for version control of Motion Behavior.
 * For details, see the SDK Manual.
 */
export declare enum MotionBehavior {
    MotionBehavior_V1 = 0,
    MotionBehavior_V2 = 1
}
/**
 * モーションクラス
 *
 * モーションのクラス。
 */
export declare class CubismMotion extends ACubismMotion {
    /**
     * インスタンスを作成する
     *
     * @param buffer motion3.jsonが読み込まれているバッファ
     * @param size バッファのサイズ
     * @param onFinishedMotionHandler モーション再生終了時に呼び出されるコールバック関数
     * @param onBeganMotionHandler モーション再生開始時に呼び出されるコールバック関数
     * @param shouldCheckMotionConsistency motion3.json整合性チェックするかどうか
     * @return 作成されたインスタンス
     */
    static create(buffer: ArrayBuffer, size: number, onFinishedMotionHandler?: FinishedMotionCallback, onBeganMotionHandler?: BeganMotionCallback, shouldCheckMotionConsistency?: boolean): CubismMotion;
    /**
     * モデルのパラメータの更新の実行
     * @param model             対象のモデル
     * @param userTimeSeconds   現在の時刻[秒]
     * @param fadeWeight        モーションの重み
     * @param motionQueueEntry  CubismMotionQueueManagerで管理されているモーション
     */
    doUpdateParameters(model: CubismModel, userTimeSeconds: number, fadeWeight: number, motionQueueEntry: CubismMotionQueueEntry): void;
    /**
     * Sets the version of the Motion Behavior.
     *
     * @param Specifies the version of the Motion Behavior.
     */
    setMotionBehavior(motionBehavior: MotionBehavior): void;
    /**
     * Gets the version of the Motion Behavior.
     *
     * @return Returns the version of the Motion Behavior.
     */
    getMotionBehavior(): MotionBehavior;
    /**
     * モーションの長さを取得する。
     *
     * @return  モーションの長さ[秒]
     */
    getDuration(): number;
    /**
     * モーションのループ時の長さを取得する。
     *
     * @return  モーションのループ時の長さ[秒]
     */
    getLoopDuration(): number;
    /**
     * パラメータに対するフェードインの時間を設定する。
     *
     * @param parameterId     パラメータID
     * @param value           フェードインにかかる時間[秒]
     */
    setParameterFadeInTime(parameterId: CubismIdHandle, value: number): void;
    /**
     * パラメータに対するフェードアウトの時間の設定
     * @param parameterId     パラメータID
     * @param value           フェードアウトにかかる時間[秒]
     */
    setParameterFadeOutTime(parameterId: CubismIdHandle, value: number): void;
    /**
     * パラメータに対するフェードインの時間の取得
     * @param    parameterId     パラメータID
     * @return   フェードインにかかる時間[秒]
     */
    getParameterFadeInTime(parameterId: CubismIdHandle): number;
    /**
     * パラメータに対するフェードアウトの時間を取得
     *
     * @param   parameterId     パラメータID
     * @return   フェードアウトにかかる時間[秒]
     */
    getParameterFadeOutTime(parameterId: CubismIdHandle): number;
    /**
     * 自動エフェクトがかかっているパラメータIDリストの設定
     * @param eyeBlinkParameterIds    自動まばたきがかかっているパラメータIDのリスト
     * @param lipSyncParameterIds     リップシンクがかかっているパラメータIDのリスト
     */
    setEffectIds(eyeBlinkParameterIds: Array<CubismIdHandle>, lipSyncParameterIds: Array<CubismIdHandle>): void;
    /**
     * コンストラクタ
     */
    constructor();
    /**
     * デストラクタ相当の処理
     */
    release(): void;
    /**
     *
     * @param motionQueueEntry
     * @param userTimeSeconds
     * @param time
     */
    updateForNextLoop(motionQueueEntry: CubismMotionQueueEntry, userTimeSeconds: number, time: number): void;
    /**
     * motion3.jsonをパースする。
     *
     * @param motionJson  motion3.jsonが読み込まれているバッファ
     * @param size        バッファのサイズ
     * @param shouldCheckMotionConsistency motion3.json整合性チェックするかどうか
     */
    parse(motionJson: ArrayBuffer, size: number, shouldCheckMotionConsistency?: boolean): void;
    /**
     * モデルのパラメータ更新
     *
     * イベント発火のチェック。
     * 入力する時間は呼ばれるモーションタイミングを０とした秒数で行う。
     *
     * @param beforeCheckTimeSeconds   前回のイベントチェック時間[秒]
     * @param motionTimeSeconds        今回の再生時間[秒]
     */
    getFiredEvent(beforeCheckTimeSeconds: number, motionTimeSeconds: number): Array<string>;
    /**
     * 透明度のカーブが存在するかどうかを確認する
     *
     * @return true  -> キーが存在する
     *          false -> キーが存在しない
     */
    isExistModelOpacity(): boolean;
    /**
     * 透明度のカーブのインデックスを返す
     *
     * @return success:透明度のカーブのインデックス
     */
    getModelOpacityIndex(): number;
    /**
     * 透明度のIdを返す
     *
     * @param index モーションカーブのインデックス
     * @return success:透明度のカーブのインデックス
     */
    getModelOpacityId(index: number): CubismIdHandle;
    /**
     * 現在時間の透明度の値を返す
     *
     * @return success:モーションの当該時間におけるOpacityの値
     */
    getModelOpacityValue(): number;
    /**
     * デバッグ用フラグを設定する
     *
     * @param debugMode デバッグモードの有効・無効
     */
    setDebugMode(debugMode: boolean): void;
    _sourceFrameRate: number;
    _loopDurationSeconds: number;
    _motionBehavior: MotionBehavior;
    _lastWeight: number;
    _motionData: CubismMotionData;
    _eyeBlinkParameterIds: Array<CubismIdHandle>;
    _lipSyncParameterIds: Array<CubismIdHandle>;
    _modelCurveIdEyeBlink: CubismIdHandle;
    _modelCurveIdLipSync: CubismIdHandle;
    _modelCurveIdOpacity: CubismIdHandle;
    _modelOpacity: number;
    private _debugMode;
}
import * as $ from './cubismmotion';
export declare namespace Live2DCubismFramework {
    const CubismMotion: typeof $.CubismMotion;
    type CubismMotion = $.CubismMotion;
}
//# sourceMappingURL=cubismmotion.d.ts.map