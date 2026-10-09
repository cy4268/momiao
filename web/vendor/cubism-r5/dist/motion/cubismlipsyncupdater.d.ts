/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
import { ICubismUpdater } from './icubismupdater';
import { CubismModel } from '../model/cubismmodel';
import { CubismIdHandle } from '../id/cubismid';
import { IParameterProvider } from './iparameterprovider';
/**
 * Updater for lip sync effects.
 * Handles the management of lip sync animation through parameter providers.
 */
export declare class CubismLipSyncUpdater extends ICubismUpdater {
    private _lipSyncIds;
    private _audioProvider;
    /**
     * Constructor
     *
     * @param lipSyncIds Array of lip sync parameter IDs
     * @param audioProvider Audio parameter provider
     */
    constructor(lipSyncIds: Array<CubismIdHandle>, audioProvider: IParameterProvider | null);
    /**
     * Constructor
     *
     * @param lipSyncIds Array of lip sync parameter IDs
     * @param audioProvider Audio parameter provider
     * @param executionOrder Order of operations
     */
    constructor(lipSyncIds: Array<CubismIdHandle>, audioProvider: IParameterProvider | null, executionOrder: number);
    /**
     * Update process.
     *
     * @param model Model to update
     * @param deltaTimeSeconds Delta time in seconds.
     */
    onLateUpdate(model: CubismModel, deltaTimeSeconds: number): void;
    /**
     * Set audio parameter provider.
     *
     * @param audioProvider Audio parameter provider to set
     */
    setAudioProvider(audioProvider: IParameterProvider | null): void;
    /**
     * Get audio parameter provider.
     *
     * @return Current audio parameter provider
     */
    getAudioProvider(): IParameterProvider | null;
}
import * as $ from './cubismlipsyncupdater';
export declare namespace Live2DCubismFramework {
    const CubismLipSyncUpdater: typeof $.CubismLipSyncUpdater;
    type CubismLipSyncUpdater = $.CubismLipSyncUpdater;
}
//# sourceMappingURL=cubismlipsyncupdater.d.ts.map