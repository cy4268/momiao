/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
import { ICubismUpdater } from './icubismupdater';
import { CubismModel } from '../model/cubismmodel';
import { CubismPose } from '../effect/cubismpose';
/**
 * Updater for pose effects.
 * Handles the management of pose animation through the CubismPose class.
 */
export declare class CubismPoseUpdater extends ICubismUpdater {
    private _pose;
    /**
     * Constructor
     *
     * @param pose CubismPose reference
     */
    constructor(pose: CubismPose);
    /**
     * Constructor
     *
     * @param pose CubismPose reference
     * @param executionOrder Order of operations
     */
    constructor(pose: CubismPose, executionOrder: number);
    /**
     * Update process.
     *
     * @param model Model to update
     * @param deltaTimeSeconds Delta time in seconds.
     */
    onLateUpdate(model: CubismModel, deltaTimeSeconds: number): void;
}
import * as $ from './cubismposeupdater';
export declare namespace Live2DCubismFramework {
    const CubismPoseUpdater: typeof $.CubismPoseUpdater;
    type CubismPoseUpdater = $.CubismPoseUpdater;
}
//# sourceMappingURL=cubismposeupdater.d.ts.map