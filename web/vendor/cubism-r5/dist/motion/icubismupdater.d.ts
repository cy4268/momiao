/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
import { CubismModel } from '../model/cubismmodel';
/**
 * Interface for listening to ICubismUpdater changes.
 */
export interface ICubismUpdaterChangeListener {
    /**
     * Called when an updater's execution order has changed.
     *
     * @param updater The updater that was changed
     */
    onUpdaterChanged(updater: ICubismUpdater): void;
}
export declare enum CubismUpdateOrder {
    CubismUpdateOrder_EyeBlink = 200,
    CubismUpdateOrder_Expression = 300,
    CubismUpdateOrder_Drag = 400,
    CubismUpdateOrder_Breath = 500,
    CubismUpdateOrder_Physics = 600,
    CubismUpdateOrder_LipSync = 700,
    CubismUpdateOrder_Pose = 800,
    CubismUpdateOrder_Max
}
/**
 * Abstract base class for motions.<br>
 * Handles the management of motion playback through the CubismUpdateScheduler.
 */
export declare abstract class ICubismUpdater {
    /**
     * Comparison function used when sorting ICubismUpdater objects.
     *
     * @param left The first ICubismUpdater object to be compared.
     * @param right The second ICubismUpdater object to be compared.
     *
     * @return negative if left should be placed before right,
     *         positive if right should be placed before left,
     *         zero if they are equal.
     */
    static sortFunction(left: ICubismUpdater, right: ICubismUpdater): number;
    private _executionOrder;
    private _changeListeners;
    /**
     * Constructor
     */
    constructor(executionOrder?: number);
    /**
     * Update process.
     *
     * @param model Model to update
     * @param deltaTimeSeconds Delta time in seconds.
     */
    abstract onLateUpdate(model: CubismModel, deltaTimeSeconds: number): void;
    getExecutionOrder(): number;
    setExecutionOrder(executionOrder: number): void;
    /**
     * Adds a listener to be notified when this updater's properties change.
     *
     * @param listener The listener to add
     */
    addChangeListener(listener: ICubismUpdaterChangeListener): void;
    /**
     * Removes a listener from the notification list.
     *
     * @param listener The listener to remove
     */
    removeChangeListener(listener: ICubismUpdaterChangeListener): void;
    /**
     * Notifies all registered listeners that this updater has changed.
     */
    private notifyChangeListeners;
}
import * as $ from './icubismupdater';
export declare namespace Live2DCubismFramework {
    const ICubismUpdater: typeof $.ICubismUpdater;
    type ICubismUpdater = $.ICubismUpdater;
    type ICubismUpdaterChangeListener = $.ICubismUpdaterChangeListener;
}
//# sourceMappingURL=icubismupdater.d.ts.map