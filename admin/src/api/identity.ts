// 身份域 REST 封装：用户、组、角色、成员关系、用户信息、头像与密码。
// 所有路径与字段以 internal/rest 的 identity 相关 handler 为准。
import {
  deleteJSON,
  getBlob,
  getJSON,
  postJSON,
  putForm,
  putJSON,
} from './client'
import type { Page, PageQuery } from './client'

/** 公开用户对象（响应不含密码、角色与禁用状态） */
export interface User {
  /** 用户 ID */
  id: string
  /** 名 */
  firstName?: string | null
  /** 姓 */
  lastName?: string | null
  /** 显示名 */
  displayName?: string | null
  /** 邮箱 */
  email?: string | null
  /** 资源地址 */
  url?: string
  /** 头像地址，无头像时缺省或为 null */
  pictureUrl?: string | null
  /** 租户 */
  tenantId?: string
}

/** 用户列表查询参数 */
export interface UserQuery extends PageQuery {
  /** 用户 ID 精确匹配 */
  id?: string
  /** 名精确匹配 */
  firstName?: string
  /** 姓精确匹配 */
  lastName?: string
  /** 显示名精确匹配 */
  displayName?: string
  /** 显示名模糊匹配（%/_ 通配） */
  displayNameLike?: string
  /** 邮箱精确匹配 */
  email?: string
  /** 邮箱模糊匹配（%/_ 通配） */
  emailLike?: string
  /** 所属组 ID */
  memberOfGroup?: string
  /** 租户 */
  tenantId?: string
}

/** 新建用户请求体 */
export interface UserCreatePayload {
  /** 用户 ID，必填 */
  id: string
  /** 名 */
  firstName?: string
  /** 姓 */
  lastName?: string
  /** 显示名 */
  displayName?: string
  /** 邮箱 */
  email?: string
  /** 初始密码 */
  password?: string
  /** 初始角色，服务端不回显 */
  roles?: string[]
  /** 租户 */
  tenantId?: string
}

/** 更新用户请求体，仅提交出现的字段 */
export interface UserUpdatePayload {
  /** 名 */
  firstName?: string
  /** 姓 */
  lastName?: string
  /** 显示名 */
  displayName?: string
  /** 邮箱 */
  email?: string
  /** 密码（管理台不使用，重置密码走独立端点） */
  password?: string
}

/** 组对象 */
export interface Group {
  /** 组 ID */
  id: string
  /** 组名 */
  name?: string | null
  /** 组类型 */
  type?: string | null
  /** 资源地址 */
  url?: string
}

/** 组列表查询参数 */
export interface GroupQuery extends PageQuery {
  /** 组 ID 精确匹配 */
  id?: string
  /** 组名精确匹配 */
  name?: string
  /** 组名模糊匹配 */
  nameLike?: string
  /** 组类型精确匹配 */
  type?: string
  /** 成员用户 ID */
  member?: string
}

/** 新建组请求体 */
export interface GroupCreatePayload {
  /** 组 ID，必填 */
  id: string
  /** 组名 */
  name?: string
  /** 组类型 */
  type?: string
  /** 租户 */
  tenantId?: string
}

/** 更新组请求体 */
export interface GroupUpdatePayload {
  /** 组名 */
  name?: string
  /** 组类型 */
  type?: string
}

/** 角色对象，权限取服务端真实字段 permissions */
export interface Role {
  /** 角色 ID */
  id: string
  /** 角色名 */
  name?: string | null
  /** 权限列表，如 read/write/admin */
  permissions?: string[] | null
  /** 租户 */
  tenantId?: string
}

/** 角色列表查询参数（服务端仅识别 start/size） */
export type RoleQuery = PageQuery

/** 新建角色请求体 */
export interface RoleCreatePayload {
  /** 角色 ID，必填 */
  id: string
  /** 角色名 */
  name?: string
  /** 权限列表 */
  permissions?: string[]
  /** 租户 */
  tenantId?: string
}

/** 更新角色请求体，PUT 为全量替换 */
export interface RoleUpdatePayload {
  /** 角色名 */
  name?: string
  /** 权限列表 */
  permissions?: string[]
  /** 租户 */
  tenantId?: string
}

/** 用户信息键（列表接口只返回键与地址，不含值） */
export interface UserInfoKey {
  /** 信息键 */
  key: string
  /** 资源地址 */
  url?: string
}

/** 用户信息键值 */
export interface UserInfo {
  /** 信息键 */
  key: string
  /** 信息值 */
  value: string
  /** 资源地址 */
  url?: string
}

/** 组成员关系响应 */
export interface Membership {
  /** 用户 ID */
  userId: string
  /** 组 ID */
  groupId: string
  /** 资源地址 */
  url?: string
}

// 构造用户资源路径。
// @param id 用户 ID
// @return 已编码的资源路径
function userPath(id: string): string {
  return `/service/identity/users/${encodeURIComponent(id)}`
}

// 构造组资源路径。
// @param id 组 ID
// @return 已编码的资源路径
function groupPath(id: string): string {
  return `/service/identity/groups/${encodeURIComponent(id)}`
}

// 构造角色资源路径。
// @param id 角色 ID
// @return 已编码的资源路径
function rolePath(id: string): string {
  return `/service/identity/roles/${encodeURIComponent(id)}`
}

// 查询用户分页列表。
// @param query 过滤、分页与排序参数
// @param signal 取消信号
// @return 用户分页结果
export function listUsers(query: UserQuery = {}, signal?: AbortSignal): Promise<Page<User>> {
  return getJSON<Page<User>>('/service/identity/users', query, signal)
}

// 查询单个用户。
// @param id 用户 ID
// @param signal 取消信号
// @return 用户对象
export function getUser(id: string, signal?: AbortSignal): Promise<User> {
  return getJSON<User>(userPath(id), undefined, signal)
}

// 创建用户。
// @param payload 用户字段，ID 必填
// @return 新建用户
export function createUser(payload: UserCreatePayload): Promise<User> {
  return postJSON<User>('/service/identity/users', payload)
}

// 更新用户公开资料（服务端仅合并提交的字段）。
// @param id 用户 ID
// @param payload 需要更新的字段
// @return 更新后的用户
export function updateUser(id: string, payload: UserUpdatePayload): Promise<User> {
  return putJSON<User>(userPath(id), payload)
}

// 删除用户。
// @param id 用户 ID
// @return 无
export function deleteUser(id: string): Promise<void> {
  return deleteJSON<void>(userPath(id))
}

// 全量设置用户角色（服务端没有角色查询接口）。
// @param id 用户 ID
// @param roles 角色 ID 列表，空数组表示清空
// @return 无
export function setUserRoles(id: string, roles: string[]): Promise<void> {
  return putJSON<void>(`${userPath(id)}/roles`, { roles })
}

// 查询用户所属组。
// @param id 用户 ID
// @param signal 取消信号
// @return 组数组
export function listUserGroups(id: string, signal?: AbortSignal): Promise<Group[]> {
  return getJSON<Group[]>(`${userPath(id)}/groups`, undefined, signal)
}

// 将用户加入组。
// @param id 用户 ID
// @param groupId 组 ID
// @return 无
export function addUserGroup(id: string, groupId: string): Promise<void> {
  return postJSON<void>(`${userPath(id)}/groups`, { groupId })
}

// 将用户移出组。
// @param id 用户 ID
// @param groupId 组 ID
// @return 无
export function removeUserGroup(id: string, groupId: string): Promise<void> {
  return deleteJSON<void>(`${userPath(id)}/groups/${encodeURIComponent(groupId)}`)
}

// 查询用户信息键列表（不含值）。
// @param id 用户 ID
// @param signal 取消信号
// @return 信息键数组
export function listUserInfoKeys(id: string, signal?: AbortSignal): Promise<UserInfoKey[]> {
  return getJSON<UserInfoKey[]>(`${userPath(id)}/info`, undefined, signal)
}

// 查询单个用户信息键值。
// @param id 用户 ID
// @param key 信息键
// @param signal 取消信号
// @return 信息键值
export function getUserInfo(id: string, key: string, signal?: AbortSignal): Promise<UserInfo> {
  return getJSON<UserInfo>(`${userPath(id)}/info/${encodeURIComponent(key)}`, undefined, signal)
}

// 加载用户全部信息键值：先取键列表，再逐个取回值。
// @param id 用户 ID
// @param signal 取消信号
// @return 按服务端键序排列的键值数组
export async function loadUserInfo(id: string, signal?: AbortSignal): Promise<UserInfo[]> {
  const keys = await listUserInfoKeys(id, signal)
  const values = await Promise.all(
    keys.map(async (entry) => {
      const detail = await getUserInfo(id, entry.key, signal)
      return { key: entry.key, value: detail.value ?? '', url: detail.url }
    }),
  )
  return values
}

// 新增用户信息键值，重复键 409。
// @param id 用户 ID
// @param key 信息键
// @param value 信息值
// @return 新建的信息键值
export function createUserInfo(id: string, key: string, value: string): Promise<UserInfo> {
  return postJSON<UserInfo>(`${userPath(id)}/info`, { key, value })
}

// 更新用户信息值。
// @param id 用户 ID
// @param key 信息键
// @param value 新值
// @return 更新后的信息键值
export function updateUserInfo(id: string, key: string, value: string): Promise<UserInfo> {
  return putJSON<UserInfo>(`${userPath(id)}/info/${encodeURIComponent(key)}`, { key, value })
}

// 删除用户信息键。
// @param id 用户 ID
// @param key 信息键
// @return 无
export function deleteUserInfo(id: string, key: string): Promise<void> {
  return deleteJSON<void>(`${userPath(id)}/info/${encodeURIComponent(key)}`)
}

// 读取用户头像二进制；无头像时服务端返回 404。
// @param id 用户 ID
// @param signal 取消信号
// @return 图片二进制
export function getUserPicture(id: string, signal?: AbortSignal): Promise<Blob> {
  return getBlob(`${userPath(id)}/picture`, undefined, signal)
}

// 上传用户头像，服务端取第一个文件 part 并从 part 读取 MIME。
// @param id 用户 ID
// @param file 图片文件
// @param signal 取消信号
// @return 无
export function setUserPicture(id: string, file: File, signal?: AbortSignal): Promise<void> {
  const form = new FormData()
  form.append('file', file, file.name)
  return putForm<void>(`${userPath(id)}/picture`, form, signal)
}

// 管理员重置用户密码（只提交新密码，不提交旧密码）。
// @param id 用户 ID
// @param newPassword 新密码
// @return 无
export function resetPassword(id: string, newPassword: string): Promise<void> {
  return putJSON<void>(`${userPath(id)}/password`, { newPassword })
}

// 查询组分页列表。
// @param query 过滤、分页与排序参数
// @param signal 取消信号
// @return 组分页结果
export function listGroups(query: GroupQuery = {}, signal?: AbortSignal): Promise<Page<Group>> {
  return getJSON<Page<Group>>('/service/identity/groups', query, signal)
}

// 查询单个组。
// @param id 组 ID
// @param signal 取消信号
// @return 组对象
export function getGroup(id: string, signal?: AbortSignal): Promise<Group> {
  return getJSON<Group>(groupPath(id), undefined, signal)
}

// 创建组。
// @param payload 组字段，ID 必填
// @return 新建组
export function createGroup(payload: GroupCreatePayload): Promise<Group> {
  return postJSON<Group>('/service/identity/groups', payload)
}

// 更新组名称与类型。
// @param id 组 ID
// @param payload 需要更新的字段
// @return 更新后的组
export function updateGroup(id: string, payload: GroupUpdatePayload): Promise<Group> {
  return putJSON<Group>(groupPath(id), payload)
}

// 删除组。
// @param id 组 ID
// @return 无
export function deleteGroup(id: string): Promise<void> {
  return deleteJSON<void>(groupPath(id))
}

// 查询组成员（服务端返回裸用户实体数组）。
// @param id 组 ID
// @param signal 取消信号
// @return 成员用户数组
export function listMembers(id: string, signal?: AbortSignal): Promise<User[]> {
  return getJSON<User[]>(`${groupPath(id)}/members`, undefined, signal)
}

// 向组添加成员，重复添加返回 409。
// @param id 组 ID
// @param userId 用户 ID
// @return 成员关系
export function addMember(id: string, userId: string): Promise<Membership> {
  return postJSON<Membership>(`${groupPath(id)}/members`, { userId })
}

// 从组移除成员，非成员返回 404。
// @param id 组 ID
// @param userId 用户 ID
// @return 无
export function removeMember(id: string, userId: string): Promise<void> {
  return deleteJSON<void>(`${groupPath(id)}/members/${encodeURIComponent(userId)}`)
}

// 查询角色分页列表（服务端不支持过滤与排序）。
// @param query 分页参数
// @param signal 取消信号
// @return 角色分页结果
export function listRoles(query: RoleQuery = {}, signal?: AbortSignal): Promise<Page<Role>> {
  return getJSON<Page<Role>>('/service/identity/roles', query, signal)
}

// 查询单个角色。
// @param id 角色 ID
// @param signal 取消信号
// @return 角色对象
export function getRole(id: string, signal?: AbortSignal): Promise<Role> {
  return getJSON<Role>(rolePath(id), undefined, signal)
}

// 创建角色。
// @param payload 角色字段，ID 必填
// @return 新建角色
export function createRole(payload: RoleCreatePayload): Promise<Role> {
  return postJSON<Role>('/service/identity/roles', payload)
}

// 全量更新角色。
// @param id 角色 ID
// @param payload 角色名与权限
// @return 更新后的角色
export function updateRole(id: string, payload: RoleUpdatePayload): Promise<Role> {
  return putJSON<Role>(rolePath(id), payload)
}

// 删除角色；内置或受保护角色由服务端返回原始错误。
// @param id 角色 ID
// @return 无
export function deleteRole(id: string): Promise<void> {
  return deleteJSON<void>(rolePath(id))
}
