/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
import { ICubismUpdater } from './icubismupdater';
import { CubismModel } from '../model/cubismmodel';
import { CubismTargetPoint } from '../math/cubismtargetpoint';
import { CubismLook } from '../effect/cubismlook';
/**
 * Updater for look effects.
 * Handles the management of dragging motion through the MotionQueueManager.
 */
export declare class CubismLookUpdater extends ICubismUpdater {
    private _look;
    private _dragManager;
    /**
     * Constructor
     *
     * @param look CubismLook reference
     * @param dragManager CubismTargetPoint reference
     */
    constructor(look: CubismLook, dragManager: CubismTargetPoint);
    /**
     * Constructor
     *
     * @param look CubismLook reference
     * @param dragManager CubismTargetPoint reference
     * @param executionOrder Order of operations
     */
    constructor(look: CubismLook, dragManager: CubismTargetPoint, executionOrder: number);
    /**
     * Update process.
     *
     * @param model Model to update
     * @param deltaTimeSeconds Delta time in seconds.
     */
    onLateUpdate(model: CubismModel, deltaTimeSeconds: number): void;
}
import * as $ from './cubismlookupdater';
export declare namespace Live2DCubismFramework {
    const CubismLookUpdater: typeof $.CubismLookUpdater;
    type CubismLookUpdater = $.CubismLookUpdater;
}
//# sourceMappingURL=cubismlookupdater.d.ts.map