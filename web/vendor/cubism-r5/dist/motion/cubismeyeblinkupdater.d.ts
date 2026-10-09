/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
import { ICubismUpdater } from './icubismupdater';
import { CubismModel } from '../model/cubismmodel';
import { CubismEyeBlink } from '../effect/cubismeyeblink';
/**
 * Updater for eye blink effects.
 * Handles the management of eye blink animation through the CubismEyeBlink class.
 */
export declare class CubismEyeBlinkUpdater extends ICubismUpdater {
    private _motionUpdated;
    private _eyeBlink;
    /**
     * Constructor
     *
     * @param motionUpdated Motion update flag reference
     * @param eyeBlink CubismEyeBlink reference
     */
    constructor(motionUpdated: () => boolean, eyeBlink: CubismEyeBlink);
    /**
     * Constructor
     *
     * @param motionUpdated Motion update flag reference
     * @param eyeBlink CubismEyeBlink reference
     * @param executionOrder Order of operations
     */
    constructor(motionUpdated: () => boolean, eyeBlink: CubismEyeBlink, executionOrder: number);
    /**
     * Update process.
     *
     * @param model Model to update
     * @param deltaTimeSeconds Delta time in seconds.
     */
    onLateUpdate(model: CubismModel, deltaTimeSeconds: number): void;
}
import * as $ from './cubismeyeblinkupdater';
export declare namespace Live2DCubismFramework {
    const CubismEyeBlinkUpdater: typeof $.CubismEyeBlinkUpdater;
    type CubismEyeBlinkUpdater = $.CubismEyeBlinkUpdater;
}
//# sourceMappingURL=cubismeyeblinkupdater.d.ts.map