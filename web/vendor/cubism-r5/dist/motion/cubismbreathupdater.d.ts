/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
import { ICubismUpdater } from './icubismupdater';
import { CubismModel } from '../model/cubismmodel';
import { CubismBreath } from '../effect/cubismbreath';
/**
 * Updater for breath effects.
 * Handles the management of breath animation through the CubismBreath class.
 */
export declare class CubismBreathUpdater extends ICubismUpdater {
    private _breath;
    /**
     * Constructor
     *
     * @param breath CubismBreath reference
     */
    constructor(breath: CubismBreath);
    /**
     * Constructor
     *
     * @param breath CubismBreath reference
     * @param executionOrder Order of operations
     */
    constructor(breath: CubismBreath, executionOrder: number);
    /**
     * Update process.
     *
     * @param model Model to update
     * @param deltaTimeSeconds Delta time in seconds.
     */
    onLateUpdate(model: CubismModel, deltaTimeSeconds: number): void;
}
import * as $ from './cubismbreathupdater';
export declare namespace Live2DCubismFramework {
    const CubismBreathUpdater: typeof $.CubismBreathUpdater;
    type CubismBreathUpdater = $.CubismBreathUpdater;
}
//# sourceMappingURL=cubismbreathupdater.d.ts.map