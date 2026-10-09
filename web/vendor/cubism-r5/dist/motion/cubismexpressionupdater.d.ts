/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
import { ICubismUpdater } from './icubismupdater';
import { CubismModel } from '../model/cubismmodel';
import { CubismExpressionMotionManager } from './cubismexpressionmotionmanager';
/**
 * Updater for expression effects.
 * Handles the management of expression motion through the CubismExpressionMotionManager.
 */
export declare class CubismExpressionUpdater extends ICubismUpdater {
    private _expressionManager;
    /**
     * Constructor
     *
     * @param expressionManager CubismExpressionMotionManager reference
     */
    constructor(expressionManager: CubismExpressionMotionManager);
    /**
     * Constructor
     *
     * @param expressionManager CubismExpressionMotionManager reference
     * @param executionOrder Order of operations
     */
    constructor(expressionManager: CubismExpressionMotionManager, executionOrder: number);
    /**
     * Update process.
     *
     * @param model Model to update
     * @param deltaTimeSeconds Delta time in seconds.
     */
    onLateUpdate(model: CubismModel, deltaTimeSeconds: number): void;
}
import * as $ from './cubismexpressionupdater';
export declare namespace Live2DCubismFramework {
    const CubismExpressionUpdater: typeof $.CubismExpressionUpdater;
    type CubismExpressionUpdater = $.CubismExpressionUpdater;
}
//# sourceMappingURL=cubismexpressionupdater.d.ts.map