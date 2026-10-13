import { ApiError } from '../api';

const messages:Record<string,string>={OPS_FORBIDDEN:'仅管理员可以操作。',OPS_OPERATION_CONFLICT:'当前业务状态不支持该操作，请刷新后核对。',OPS_CAPABILITY_UNAVAILABLE:'该运营能力暂不可用。',LAST_SUPER_ADMIN_REQUIRED:'必须保留至少一位有效的最高管理员。'};
export function opsError(error:unknown):string{return error instanceof ApiError?(Object.hasOwn(messages,error.code)?messages[error.code]:error.message):error instanceof Error?error.message:'运营请求失败，请重新加载。';}
