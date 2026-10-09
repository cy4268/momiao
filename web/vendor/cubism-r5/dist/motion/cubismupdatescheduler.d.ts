/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
import { ICubismUpdater, ICubismUpdaterChangeListener } from './icubismupdater';
import { CubismModel } from '../model/cubismmodel';
/**
 * Scheduler for managing and updating ICubismUpdater instances.
 * Handles the management of update order and execution through a sorted list.
 */
export declare class CubismUpdateScheduler implements ICubismUpdaterChangeListener {
    private _cubismUpdatableList;
    private _needsSort;
    /**
     * Constructor
     */
    constructor();
    /**
     * Destructor equivalent - releases all updaters and removes listeners
     */
    release(): void;
    /**
     * Adds ICubismUpdater to the update list.
     * The list will be automatically sorted by execution order before the next update.
     *
     * @param updatable The ICubismUpdater instance to be added.
     */
    addUpdatableList(updatable: ICubismUpdater): void;
    /**
     * Removes ICubismUpdater from the update list.
     *
     * @param updatable The ICubismUpdater instance to be removed.
     * @return true if the updater was found and removed, false otherwise.
     */
    removeUpdatableList(updatable: ICubismUpdater): boolean;
    /**
     * Sorts the update list using the ICubismUpdater sort function.
     */
    sortUpdatableList(): void;
    /**
     * Updates every element in the list.
     * The list is automatically sorted by execution order before execution.
     *
     * @param model Model to update
     * @param deltaTimeSeconds Delta time in seconds.
     */
    onLateUpdate(model: CubismModel, deltaTimeSeconds: number): void;
    /**
     * Gets the number of updaters in the list.
     *
     * @return Number of updaters
     */
    getUpdatableCount(): number;
    /**
     * Gets the updater at the specified index.
     *
     * @param index Index of the updater to retrieve
     * @return The updater at the specified index, or null if index is out of bounds
     */
    getUpdatable(index: number): ICubismUpdater | null;
    /**
     * Checks if the specified updater exists in the list.
     *
     * @param updatable The updater to check for
     * @return true if the updater exists in the list, false otherwise
     */
    hasUpdatable(updatable: ICubismUpdater): boolean;
    /**
     * Clears all updaters from the list.
     */
    clearUpdatableList(): void;
    /**
     * Called when an updater's execution order has changed.
     * Marks the list for re-sorting.
     *
     * @param updater The updater that was changed
     */
    onUpdaterChanged(updater: ICubismUpdater): void;
}
import * as $ from './cubismupdatescheduler';
export declare namespace Live2DCubismFramework {
    const CubismUpdateScheduler: typeof $.CubismUpdateScheduler;
    type CubismUpdateScheduler = $.CubismUpdateScheduler;
}
//# sourceMappingURL=cubismupdatescheduler.d.ts.map