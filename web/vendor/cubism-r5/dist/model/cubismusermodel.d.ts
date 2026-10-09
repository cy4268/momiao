/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
import { CubismBreath } from '../effect/cubismbreath';
import { CubismEyeBlink } from '../effect/cubismeyeblink';
import { CubismPose } from '../effect/cubismpose';
import { ICubismModelSetting } from '../icubismmodelsetting';
import { CubismIdHandle } from '../id/cubismid';
import { CubismModelMatrix } from '../math/cubismmodelmatrix';
import { CubismTargetPoint } from '../math/cubismtargetpoint';
import { ACubismMotion, BeganMotionCallback, FinishedMotionCallback } from '../motion/acubismmotion';
import { CubismExpressionMotionManager } from '../motion/cubismexpressionmotionmanager';
import { CubismMotion } from '../motion/cubismmotion';
import { CubismMotionManager } from '../motion/cubismmotionmanager';
import { CubismMotionQueueManager } from '../motion/cubismmotionqueuemanager';
import { CubismPhysics } from '../physics/cubismphysics';
import { CubismRenderer_WebGL } from '../rendering/cubismrenderer_webgl';
import { CubismMoc } from './cubismmoc';
import { CubismModel } from './cubismmodel';
import { CubismModelUserData } from './cubismmodeluserdata';
/**
 * ユーザーが実際に使用するモデル
 *
 * ユーザーが実際に使用するモデルの基底クラス。これを継承してユーザーが実装する。
 */
export declare class CubismUserModel {
    /**
     * 初期化状態の取得
     *
     * 初期化されている状態か？
     *
     * @return true     初期化されている
     * @return false    初期化されていない
     */
    isInitialized(): boolean;
    /**
     * 初期化状態の設定
     *
     * 初期化状態を設定する。
     *
     * @param v 初期化状態
     */
    setInitialized(v: boolean): void;
    /**
     * 更新状態の取得
     *
     * 更新されている状態か？
     *
     * @return true     更新されている
     * @return false    更新されていない
     */
    isUpdating(): boolean;
    /**
     * 更新状態の設定
     *
     * 更新状態を設定する
     *
     * @param v 更新状態
     */
    setUpdating(v: boolean): void;
    /**
     * マウスドラッグ情報の設定
     *
     * @param ドラッグしているカーソルのX位置
     * @param ドラッグしているカーソルのY位置
     */
    setDragging(x: number, y: number): void;
    /**
     * モデル行列を取得する
     * @return モデル行列
     */
    getModelMatrix(): CubismModelMatrix;
    /**
     * モデルを描画したバッファを設定する
     *
     * @param width モデルを描画したバッファの幅
     * @param height モデルを描画したバッファの高さ
     */
    setRenderTargetSize(width: number, height: number): void;
    /**
     * 不透明度の設定
     *
     * @param a 不透明度
     */
    setOpacity(a: number): void;
    /**
     * 不透明度の取得
     *
     * @return 不透明度
     */
    getOpacity(): number;
    /**
     * モデルデータを読み込む
     *
     * @param buffer    moc3ファイルが読み込まれているバッファ
     */
    loadModel(buffer: ArrayBuffer, shouldCheckMocConsistency?: boolean): void;
    /**
     * モーションデータを読み込む
     * @param buffer motion3.jsonファイルが読み込まれているバッファ
     * @param size バッファのサイズ
     * @param name モーションの名前
     * @param onFinishedMotionHandler モーション再生終了時に呼び出されるコールバック関数
     * @param onBeganMotionHandler モーション再生開始時に呼び出されるコールバック関数
     * @param modelSetting モデル設定
     * @param group モーショングループ名
     * @param index モーションインデックス
     * @param shouldCheckMotionConsistency motion3.json整合性チェックするかどうか
     * @return モーションクラス
     */
    loadMotion(buffer: ArrayBuffer, size: number, name: string, onFinishedMotionHandler?: FinishedMotionCallback, onBeganMotionHandler?: BeganMotionCallback, modelSetting?: ICubismModelSetting, group?: string, index?: number, shouldCheckMotionConsistency?: boolean): CubismMotion;
    /**
     * 表情データの読み込み
     * @param buffer expファイルが読み込まれているバッファ
     * @param size バッファのサイズ
     * @param name 表情の名前
     */
    loadExpression(buffer: ArrayBuffer, size: number, name: string): ACubismMotion;
    /**
     * ポーズデータの読み込み
     * @param buffer pose3.jsonが読み込まれているバッファ
     * @param size バッファのサイズ
     */
    loadPose(buffer: ArrayBuffer, size: number): void;
    /**
     * モデルに付属するユーザーデータを読み込む
     * @param buffer userdata3.jsonが読み込まれているバッファ
     * @param size バッファのサイズ
     */
    loadUserData(buffer: ArrayBuffer, size: number): void;
    /**
     * 物理演算データの読み込み
     * @param buffer  physics3.jsonが読み込まれているバッファ
     * @param size    バッファのサイズ
     */
    loadPhysics(buffer: ArrayBuffer, size: number): void;
    /**
     * 当たり判定の取得
     * @param drawableId 検証したいDrawableのID
     * @param pointX X位置
     * @param pointY Y位置
     * @return true ヒットしている
     * @return false ヒットしていない
     */
    isHit(drawableId: CubismIdHandle, pointX: number, pointY: number): boolean;
    /**
     * モデルの取得
     * @return モデル
     */
    getModel(): CubismModel;
    /**
     * 読み込めないMocファイルの.moc3 Versionを取得
     * @param mocBytes 読み込めないMocファイルのバイト配列
     * @returns .moc3 Version番号
     */
    getMocVersionFromBuffer(mocBytes: ArrayBuffer): number;
    /**
     * レンダラの取得
     * @return レンダラ
     */
    getRenderer(): CubismRenderer_WebGL;
    /**
     * レンダラを作成して初期化を実行する
     * @param width レンダリングする幅
     * @param height レンダリングする高さ
     * @param maskBufferCount バッファの生成数
     */
    createRenderer(width: number, height: number, maskBufferCount?: number): void;
    /**
     * レンダラの解放
     */
    deleteRenderer(): void;
    /**
     * イベント発火時の標準処理
     *
     * Eventが再生処理時にあった場合の処理をする。
     * 継承で上書きすることを想定している。
     * 上書きしない場合はログ出力をする。
     *
     * @param eventValue 発火したイベントの文字列データ
     */
    motionEventFired(eventValue: string): void;
    /**
     * イベント用のコールバック
     *
     * CubismMotionQueueManagerにイベント用に登録するためのCallback。
     * CubismUserModelの継承先のEventFiredを呼ぶ。
     *
     * @param caller 発火したイベントを管理していたモーションマネージャー、比較用
     * @param eventValue 発火したイベントの文字列データ
     * @param customData CubismUserModelを継承したインスタンスを想定
     */
    static cubismDefaultMotionEventCallback(caller: CubismMotionQueueManager, eventValue: string, customData: CubismUserModel): void;
    /**
     * コンストラクタ
     */
    constructor();
    /**
     * デストラクタに相当する処理
     */
    release(): void;
    protected _moc: CubismMoc;
    protected _model: CubismModel;
    protected _motionManager: CubismMotionManager;
    protected _expressionManager: CubismExpressionMotionManager;
    protected _eyeBlink: CubismEyeBlink;
    protected _breath: CubismBreath;
    protected _modelMatrix: CubismModelMatrix;
    protected _pose: CubismPose;
    protected _dragManager: CubismTargetPoint;
    protected _physics: CubismPhysics;
    protected _modelUserData: CubismModelUserData;
    protected _initialized: boolean;
    protected _updating: boolean;
    protected _opacity: number;
    protected _mocConsistency: boolean;
    protected _motionConsistency: boolean;
    protected _debugMode: boolean;
    private _renderer;
}
import * as $ from './cubismusermodel';
export declare namespace Live2DCubismFramework {
    const CubismUserModel: typeof $.CubismUserModel;
    type CubismUserModel = $.CubismUserModel;
}
//# sourceMappingURL=cubismusermodel.d.ts.map