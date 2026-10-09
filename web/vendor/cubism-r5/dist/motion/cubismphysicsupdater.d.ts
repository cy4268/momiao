/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
import { ICubismUpdater } from './icubismupdater';
import { CubismModel } from '../model/cubismmodel';
import { CubismPhysics } from '../physics/cubismphysics';
/**
 * Updater for physics effects.
 * Handles the management of physics simulation through the CubismPhysics class.
 */
export declare class CubismPhysicsUpdater extends ICubismUpdater {
    private _physics;
    /**
     * Constructor
     *
     * @param physics CubismPhysics reference
     */
    constructor(physics: CubismPhysics);
    /**
     * Constructor
     *
     * @param physics CubismPhysics reference
     * @param executionOrder Order of operations
     */
    constructor(physics: CubismPhysics, executionOrder: number);
    /**
     * Update process.
     *
     * @param model Model to update
     * @param deltaTimeSeconds Delta time in seconds.
     */
    onLateUpdate(model: CubismModel, deltaTimeSeconds: number): void;
}
import * as $ from './cubismphysicsupdater';
export declare namespace Live2DCubismFramework {
    const CubismPhysicsUpdater: typeof $.CubismPhysicsUpdater;
    type CubismPhysicsUpdater = $.CubismPhysicsUpdater;
}
//# sourceMappingURL=cubismphysicsupdater.d.ts.map