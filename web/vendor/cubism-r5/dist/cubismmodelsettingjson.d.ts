/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
import { ICubismModelSetting } from './icubismmodelsetting';
import { CubismIdHandle } from './id/cubismid';
import { CubismJson, Value } from './utils/cubismjson';
export declare enum FrequestNode {
    FrequestNode_Groups = 0,// getRoot().getValueByString(Groups)
    FrequestNode_Moc = 1,// getRoot().getValueByString(FileReferences).getValueByString(Moc)
    FrequestNode_Motions = 2,// getRoot().getValueByString(FileReferences).getValueByString(Motions)
    FrequestNode_Expressions = 3,// getRoot().getValueByString(FileReferences).getValueByString(Expressions)
    FrequestNode_Textures = 4,// getRoot().getValueByString(FileReferences).getValueByString(Textures)
    FrequestNode_Physics = 5,// getRoot().getValueByString(FileReferences).getValueByString(Physics)
    FrequestNode_Pose = 6,// getRoot().getValueByString(FileReferences).getValueByString(Pose)
    FrequestNode_HitAreas = 7
}
/**
 * Model3Jsonパーサー
 *
 * model3.jsonファイルをパースして値を取得する
 */
export declare class CubismModelSettingJson extends ICubismModelSetting {
    /**
     * 引数付きコンストラクタ
     *
     * @param buffer    Model3Jsonをバイト配列として読み込んだデータバッファ
     * @param size      Model3Jsonのデータサイズ
     */
    constructor(buffer: ArrayBuffer, size: number);
    /**
     * デストラクタ相当の処理
     */
    release(): void;
    /**
     * CubismJsonオブジェクトを取得する
     *
     * @return CubismJson
     */
    getJson(): CubismJson;
    /**
     * Mocファイルの名前を取得する
     * @return Mocファイルの名前
     */
    getModelFileName(): string;
    /**
     * モデルが使用するテクスチャの数を取得する
     * テクスチャの数
     */
    getTextureCount(): number;
    /**
     * テクスチャが配置されたディレクトリの名前を取得する
     * @return テクスチャが配置されたディレクトリの名前
     */
    getTextureDirectory(): string;
    /**
     * モデルが使用するテクスチャの名前を取得する
     * @param index 配列のインデックス値
     * @return テクスチャの名前
     */
    getTextureFileName(index: number): string;
    /**
     * モデルに設定された当たり判定の数を取得する
     * @return モデルに設定された当たり判定の数
     */
    getHitAreasCount(): number;
    /**
     * 当たり判定に設定されたIDを取得する
     *
     * @param index 配列のindex
     * @return 当たり判定に設定されたID
     */
    getHitAreaId(index: number): CubismIdHandle;
    /**
     * 当たり判定に設定された名前を取得する
     * @param index 配列のインデックス値
     * @return 当たり判定に設定された名前
     */
    getHitAreaName(index: number): string;
    /**
     * 物理演算設定ファイルの名前を取得する
     * @return 物理演算設定ファイルの名前
     */
    getPhysicsFileName(): string;
    /**
     * パーツ切り替え設定ファイルの名前を取得する
     * @return パーツ切り替え設定ファイルの名前
     */
    getPoseFileName(): string;
    /**
     * 表情設定ファイルの数を取得する
     * @return 表情設定ファイルの数
     */
    getExpressionCount(): number;
    /**
     * 表情設定ファイルを識別する名前（別名）を取得する
     * @param index 配列のインデックス値
     * @return 表情の名前
     */
    getExpressionName(index: number): string;
    /**
     * 表情設定ファイルの名前を取得する
     * @param index 配列のインデックス値
     * @return 表情設定ファイルの名前
     */
    getExpressionFileName(index: number): string;
    /**
     * モーショングループの数を取得する
     * @return モーショングループの数
     */
    getMotionGroupCount(): number;
    /**
     * モーショングループの名前を取得する
     * @param index 配列のインデックス値
     * @return モーショングループの名前
     */
    getMotionGroupName(index: number): string;
    /**
     * モーショングループに含まれるモーションの数を取得する
     * @param groupName モーショングループの名前
     * @return モーショングループの数
     */
    getMotionCount(groupName: string): number;
    /**
     * グループ名とインデックス値からモーションファイル名を取得する
     * @param groupName モーショングループの名前
     * @param index     配列のインデックス値
     * @return モーションファイルの名前
     */
    getMotionFileName(groupName: string, index: number): string;
    /**
     * モーションに対応するサウンドファイルの名前を取得する
     * @param groupName モーショングループの名前
     * @param index 配列のインデックス値
     * @return サウンドファイルの名前
     */
    getMotionSoundFileName(groupName: string, index: number): string;
    /**
     * モーション開始時のフェードイン処理時間を取得する
     * @param groupName モーショングループの名前
     * @param index 配列のインデックス値
     * @return フェードイン処理時間[秒]
     */
    getMotionFadeInTimeValue(groupName: string, index: number): number;
    /**
     * モーション終了時のフェードアウト処理時間を取得する
     * @param groupName モーショングループの名前
     * @param index 配列のインデックス値
     * @return フェードアウト処理時間[秒]
     */
    getMotionFadeOutTimeValue(groupName: string, index: number): number;
    /**
     * ユーザーデータのファイル名を取得する
     * @return ユーザーデータのファイル名
     */
    getUserDataFile(): string;
    /**
     * レイアウト情報を取得する
     * @param outLayoutMap Mapクラスのインスタンス
     * @return true レイアウト情報が存在する
     * @return false レイアウト情報が存在しない
     */
    getLayoutMap(outLayoutMap: Map<string, number>): boolean;
    /**
     * 目パチに関連付けられたパラメータの数を取得する
     * @return 目パチに関連付けられたパラメータの数
     */
    getEyeBlinkParameterCount(): number;
    /**
     * 目パチに関連付けられたパラメータのIDを取得する
     * @param index 配列のインデックス値
     * @return パラメータID
     */
    getEyeBlinkParameterId(index: number): CubismIdHandle;
    /**
     * リップシンクに関連付けられたパラメータの数を取得する
     * @return リップシンクに関連付けられたパラメータの数
     */
    getLipSyncParameterCount(): number;
    /**
     * リップシンクに関連付けられたパラメータの数を取得する
     * @param index 配列のインデックス値
     * @return パラメータID
     */
    getLipSyncParameterId(index: number): CubismIdHandle;
    /**
     * モデルファイルのキーが存在するかどうかを確認する
     * @return true キーが存在する
     * @return false キーが存在しない
     */
    protected isExistModelFile(): boolean;
    /**
     * テクスチャファイルのキーが存在するかどうかを確認する
     * @return true キーが存在する
     * @return false キーが存在しない
     */
    protected isExistTextureFiles(): boolean;
    /**
     * 当たり判定のキーが存在するかどうかを確認する
     * @return true キーが存在する
     * @return false キーが存在しない
     */
    protected isExistHitAreas(): boolean;
    /**
     * 物理演算ファイルのキーが存在するかどうかを確認する
     * @return true キーが存在する
     * @return false キーが存在しない
     */
    protected isExistPhysicsFile(): boolean;
    /**
     * ポーズ設定ファイルのキーが存在するかどうかを確認する
     * @return true キーが存在する
     * @return false キーが存在しない
     */
    protected isExistPoseFile(): boolean;
    /**
     * 表情設定ファイルのキーが存在するかどうかを確認する
     * @return true キーが存在する
     * @return false キーが存在しない
     */
    protected isExistExpressionFile(): boolean;
    /**
     * モーショングループのキーが存在するかどうかを確認する
     * @return true キーが存在する
     * @return false キーが存在しない
     */
    protected isExistMotionGroups(): boolean;
    /**
     * 引数で指定したモーショングループのキーが存在するかどうかを確認する
     * @param groupName  グループ名
     * @return true キーが存在する
     * @return false キーが存在しない
     */
    protected isExistMotionGroupName(groupName: string): boolean;
    /**
     * 引数で指定したモーションに対応するサウンドファイルのキーが存在するかどうかを確認する
     * @param groupName  グループ名
     * @param index 配列のインデックス値
     * @return true キーが存在する
     * @return false キーが存在しない
     */
    protected isExistMotionSoundFile(groupName: string, index: number): boolean;
    /**
     * 引数で指定したモーションに対応するフェードイン時間のキーが存在するかどうかを確認する
     * @param groupName  グループ名
     * @param index 配列のインデックス値
     * @return true キーが存在する
     * @return false キーが存在しない
     */
    protected isExistMotionFadeIn(groupName: string, index: number): boolean;
    /**
     * 引数で指定したモーションに対応するフェードアウト時間のキーが存在するかどうかを確認する
     * @param groupName  グループ名
     * @param index 配列のインデックス値
     * @return true キーが存在する
     * @return false キーが存在しない
     */
    protected isExistMotionFadeOut(groupName: string, index: number): boolean;
    /**
     * UserDataのファイル名が存在するかどうかを確認する
     * @return true キーが存在する
     * @return false キーが存在しない
     */
    protected isExistUserDataFile(): boolean;
    /**
     * 目ぱちに対応付けられたパラメータが存在するかどうかを確認する
     * @return true キーが存在する
     * @return false キーが存在しない
     */
    protected isExistEyeBlinkParameters(): boolean;
    /**
     * リップシンクに対応付けられたパラメータが存在するかどうかを確認する
     * @return true キーが存在する
     * @return false キーが存在しない
     */
    protected isExistLipSyncParameters(): boolean;
    protected _json: CubismJson;
    protected _jsonValue: Array<Value>;
    /**
     * Model3Jsonのキー文字列
     */
    protected readonly version = "Version";
    protected readonly fileReferences = "FileReferences";
    protected readonly groups = "Groups";
    protected readonly layout = "Layout";
    protected readonly hitAreas = "HitAreas";
    protected readonly moc = "Moc";
    protected readonly textures = "Textures";
    protected readonly physics = "Physics";
    protected readonly pose = "Pose";
    protected readonly expressions = "Expressions";
    protected readonly motions = "Motions";
    protected readonly userData = "UserData";
    protected readonly name = "Name";
    protected readonly filePath = "File";
    protected readonly id = "Id";
    protected readonly ids = "Ids";
    protected readonly target = "Target";
    protected readonly idle = "Idle";
    protected readonly tapBody = "TapBody";
    protected readonly pinchIn = "PinchIn";
    protected readonly pinchOut = "PinchOut";
    protected readonly shake = "Shake";
    protected readonly flickHead = "FlickHead";
    protected readonly parameter = "Parameter";
    protected readonly soundPath = "Sound";
    protected readonly fadeInTime = "FadeInTime";
    protected readonly fadeOutTime = "FadeOutTime";
    protected readonly centerX = "CenterX";
    protected readonly centerY = "CenterY";
    protected readonly x = "X";
    protected readonly y = "Y";
    protected readonly width = "Width";
    protected readonly height = "Height";
    protected readonly lipSync = "LipSync";
    protected readonly eyeBlink = "EyeBlink";
    protected readonly initParameter = "init_param";
    protected readonly initPartsVisible = "init_parts_visible";
    protected readonly val = "val";
}
import * as $ from './cubismmodelsettingjson';
export declare namespace Live2DCubismFramework {
    const CubismModelSettingJson: typeof $.CubismModelSettingJson;
    type CubismModelSettingJson = $.CubismModelSettingJson;
    const FrequestNode: typeof $.FrequestNode;
    type FrequestNode = $.FrequestNode;
}
//# sourceMappingURL=cubismmodelsettingjson.d.ts.map