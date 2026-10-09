/**
 * Copyright(c) Live2D Inc. All rights reserved.
 *
 * Use of this source code is governed by the Live2D Open Software license
 * that can be found at https://www.live2d.com/eula/live2d-open-software-license-agreement_en.html.
 */
/**
 * Interface class for providing parameter values.<br>
 * Defines the base interface for classes that supply parameter values to the model.
 */
export declare abstract class IParameterProvider {
    /**
     * Constructor
     */
    constructor();
    /**
     * Update process.
     *
     * @param deltaTimeSeconds Delta time in seconds (optional).
     *
     * @return true if the update is successful.
     */
    abstract update(deltaTimeSeconds?: number): boolean;
    /**
     * Retrieves the current value of the parameter.
     *
     * @return The parameter value as a floating-point number.
     */
    abstract getParameter(): number;
}
import * as $ from './iparameterprovider';
export declare namespace Live2DCubismFramework {
    const IParameterProvider: typeof $.IParameterProvider;
    type IParameterProvider = $.IParameterProvider;
}
//# sourceMappingURL=iparameterprovider.d.ts.map