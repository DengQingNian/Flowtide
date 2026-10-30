// 身份 API 模块测试：覆盖每个函数的 URL、方法、请求体与错误映射。
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  addMember,
  addUserGroup,
  createGroup,
  createRole,
  createUser,
  createUserInfo,
  deleteGroup,
  deleteRole,
  deleteUser,
  deleteUserInfo,
  getGroup,
  getRole,
  getUser,
  getUserInfo,
  getUserPicture,
  listGroups,
  listMembers,
  listRoles,
  listUserGroups,
  listUserInfoKeys,
  listUsers,
  loadUserInfo,
  removeMember,
  removeUserGroup,
  resetPassword,
  setUserPicture,
  setUserRoles,
  updateGroup,
  updateRole,
  updateUser,
  updateUserInfo,
} from '@/api/identity'
import { ApiError } from '@/api/client'
import { mockFetch, pageOf } from '../helpers'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('身份 API', () => {
  it('listUsers 按过滤与分页参数请求列表', async () => {
    const calls = mockFetch([
      {
        path: /^\/service\/identity\/users\?/,
        body: pageOf([{ id: 'alice', displayName: 'Alice' }], 1),
      },
    ])
    const result = await listUsers({ id: 'alice', displayNameLike: 'A%', emailLike: 'a@example.com', start: 50, size: 50, sort: 'id', order: 'asc' })
    expect(result.total).toBe(1)
    expect(calls[0].method).toBe('GET')
    expect(calls[0].url).toContain('/service/identity/users?')
    expect(calls[0].url).toContain('id=alice')
    expect(calls[0].url).toContain('displayNameLike=A%25')
    expect(calls[0].url).toContain('start=50')
    expect(calls[0].url).toContain('sort=id')
  })

  it('getUser 请求单个用户', async () => {
    const calls = mockFetch([{ path: '/service/identity/users/alice', body: { id: 'alice' } }])
    const user = await getUser('alice')
    expect(user.id).toBe('alice')
    expect(calls[0].method).toBe('GET')
    expect(calls[0].url).toBe('/service/identity/users/alice')
  })

  it('createUser 提交新建用户请求体', async () => {
    const calls = mockFetch([{ path: '/service/identity/users', method: 'POST', status: 201, body: { id: 'alice' } }])
    await createUser({ id: 'alice', displayName: 'Alice', password: 'secret', roles: ['reader'] })
    expect(calls[0].method).toBe('POST')
    expect(calls[0].body).toEqual({ id: 'alice', displayName: 'Alice', password: 'secret', roles: ['reader'] })
  })

  it('updateUser 提交局部资料', async () => {
    const calls = mockFetch([{ path: '/service/identity/users/alice', method: 'PUT', body: { id: 'alice' } }])
    await updateUser('alice', { displayName: 'Alice', email: '' })
    expect(calls[0].method).toBe('PUT')
    expect(calls[0].body).toEqual({ displayName: 'Alice', email: '' })
  })

  it('deleteUser 使用 DELETE 且无响应正文', async () => {
    const calls = mockFetch([{ path: '/service/identity/users/alice', method: 'DELETE', status: 204 }])
    await expect(deleteUser('alice')).resolves.toBeUndefined()
    expect(calls[0].method).toBe('DELETE')
  })

  it('setUserRoles 全量提交角色数组', async () => {
    const calls = mockFetch([{ path: '/service/identity/users/alice/roles', method: 'PUT', status: 204 }])
    await setUserRoles('alice', ['admin', 'reader'])
    expect(calls[0].body).toEqual({ roles: ['admin', 'reader'] })
  })

  it('listUserGroups 请求用户所属组', async () => {
    const calls = mockFetch([{ path: '/service/identity/users/alice/groups', body: [{ id: 'g1' }] }])
    const groups = await listUserGroups('alice')
    expect(groups).toEqual([{ id: 'g1' }])
    expect(calls[0].method).toBe('GET')
  })

  it('addUserGroup 提交 groupId', async () => {
    const calls = mockFetch([{ path: '/service/identity/users/alice/groups', method: 'POST', status: 204 }])
    await addUserGroup('alice', 'g1')
    expect(calls[0].body).toEqual({ groupId: 'g1' })
  })

  it('removeUserGroup 编码路径中的组 ID', async () => {
    const calls = mockFetch([{ path: /^\/service\/identity\/users\/alice\/groups\/g%2F1$/, method: 'DELETE', status: 204 }])
    await removeUserGroup('alice', 'g/1')
    expect(calls[0].url).toBe('/service/identity/users/alice/groups/g%2F1')
  })

  it('listUserInfoKeys 只请求键列表', async () => {
    const calls = mockFetch([{ path: '/service/identity/users/alice/info', body: [{ key: 'dept' }] }])
    expect(await listUserInfoKeys('alice')).toEqual([{ key: 'dept' }])
    expect(calls[0].method).toBe('GET')
  })

  it('getUserInfo 请求单个键值', async () => {
    const calls = mockFetch([{ path: '/service/identity/users/alice/info/dept', body: { key: 'dept', value: '研发' } }])
    const info = await getUserInfo('alice', 'dept')
    expect(info.value).toBe('研发')
    expect(calls[0].url).toBe('/service/identity/users/alice/info/dept')
  })

  it('loadUserInfo 先取键再逐键取值', async () => {
    const calls = mockFetch([
      { path: /^\/service\/identity\/users\/alice\/info$/, body: [{ key: 'dept' }, { key: 'title' }] },
      { path: '/service/identity/users/alice/info/dept', body: { key: 'dept', value: '研发' } },
      { path: '/service/identity/users/alice/info/title', body: { key: 'title', value: '工程师' } },
    ])
    const entries = await loadUserInfo('alice')
    expect(entries).toEqual([
      { key: 'dept', value: '研发', url: undefined },
      { key: 'title', value: '工程师', url: undefined },
    ])
    expect(calls).toHaveLength(3)
  })

  it('loadUserInfo 无键时返回空数组', async () => {
    mockFetch([{ path: /^\/service\/identity\/users\/alice\/info$/, body: [] }])
    await expect(loadUserInfo('alice')).resolves.toEqual([])
  })

  it('createUserInfo 提交键值', async () => {
    const calls = mockFetch([{ path: '/service/identity/users/alice/info', method: 'POST', status: 201, body: { key: 'dept', value: '研发' } }])
    await createUserInfo('alice', 'dept', '研发')
    expect(calls[0].body).toEqual({ key: 'dept', value: '研发' })
  })

  it('updateUserInfo 提交键与值', async () => {
    const calls = mockFetch([{ path: '/service/identity/users/alice/info/dept', method: 'PUT', body: { key: 'dept', value: '测试' } }])
    await updateUserInfo('alice', 'dept', '测试')
    expect(calls[0].body).toEqual({ key: 'dept', value: '测试' })
  })

  it('deleteUserInfo 编码路径中的键', async () => {
    const calls = mockFetch([{ path: /^\/service\/identity\/users\/alice\/info\/a%2Fb$/, method: 'DELETE', status: 204 }])
    await deleteUserInfo('alice', 'a/b')
    expect(calls[0].url).toBe('/service/identity/users/alice/info/a%2Fb')
  })

  it('getUserPicture 以二进制读取头像', async () => {
    const calls = mockFetch([{ path: '/service/identity/users/alice/picture', text: 'PNGDATA', contentType: 'image/png' }])
    const blob = await getUserPicture('alice')
    expect(blob).toBeInstanceOf(Blob)
    expect(blob.type).toBe('image/png')
    expect(blob.size).toBe(7)
    expect(calls[0].method).toBe('GET')
  })

  it('setUserPicture 以 multipart 上传首个文件', async () => {
    const calls = mockFetch([{ path: '/service/identity/users/alice/picture', method: 'PUT', status: 204 }])
    const file = new File(['avatar'], 'avatar.png', { type: 'image/png' })
    await setUserPicture('alice', file)
    expect(calls[0].method).toBe('PUT')
    expect(calls[0].body).toBeInstanceOf(FormData)
    const form = calls[0].body as FormData
    expect((form.get('file') as File).name).toBe('avatar.png')
  })

  it('resetPassword 只提交新密码', async () => {
    const calls = mockFetch([{ path: '/service/identity/users/alice/password', method: 'PUT', status: 204 }])
    await resetPassword('alice', 'new-secret')
    expect(calls[0].body).toEqual({ newPassword: 'new-secret' })
  })

  it('listGroups 请求组列表', async () => {
    const calls = mockFetch([{ path: /^\/service\/identity\/groups\?/, body: pageOf([{ id: 'g1' }]) }])
    const result = await listGroups({ id: 'g1', nameLike: '审%', type: 'assignment', start: 0, size: 50, sort: 'name', order: 'desc' })
    expect(result.data[0].id).toBe('g1')
    expect(calls[0].url).toContain('nameLike=')
    expect(calls[0].url).toContain('sort=name')
  })

  it('getGroup 请求单个组', async () => {
    const calls = mockFetch([{ path: '/service/identity/groups/g1', body: { id: 'g1', name: '审批组' } }])
    expect((await getGroup('g1')).name).toBe('审批组')
    expect(calls[0].method).toBe('GET')
  })

  it('createGroup 提交新建组', async () => {
    const calls = mockFetch([{ path: '/service/identity/groups', method: 'POST', status: 201, body: { id: 'g1' } }])
    await createGroup({ id: 'g1', name: '审批组', type: 'assignment' })
    expect(calls[0].body).toEqual({ id: 'g1', name: '审批组', type: 'assignment' })
  })

  it('updateGroup 提交局部名称与类型', async () => {
    const calls = mockFetch([{ path: '/service/identity/groups/g1', method: 'PUT', body: { id: 'g1' } }])
    await updateGroup('g1', { name: '新名称', type: '' })
    expect(calls[0].body).toEqual({ name: '新名称', type: '' })
  })

  it('deleteGroup 使用 DELETE', async () => {
    const calls = mockFetch([{ path: '/service/identity/groups/g1', method: 'DELETE', status: 204 }])
    await expect(deleteGroup('g1')).resolves.toBeUndefined()
    expect(calls[0].method).toBe('DELETE')
  })

  it('listMembers 返回裸用户数组', async () => {
    const calls = mockFetch([{ path: '/service/identity/groups/g1/members', body: [{ id: 'alice' }] }])
    const members = await listMembers('g1')
    expect(members).toEqual([{ id: 'alice' }])
    expect(calls[0].method).toBe('GET')
  })

  it('addMember 提交 userId', async () => {
    const calls = mockFetch([{ path: '/service/identity/groups/g1/members', method: 'POST', status: 201, body: { userId: 'alice', groupId: 'g1' } }])
    const membership = await addMember('g1', 'alice')
    expect(membership.userId).toBe('alice')
    expect(calls[0].body).toEqual({ userId: 'alice' })
  })

  it('removeMember 编码用户 ID', async () => {
    const calls = mockFetch([{ path: /^\/service\/identity\/groups\/g1\/members\/a%2Fb$/, method: 'DELETE', status: 204 }])
    await removeMember('g1', 'a/b')
    expect(calls[0].url).toBe('/service/identity/groups/g1/members/a%2Fb')
  })

  it('listRoles 请求角色分页', async () => {
    const calls = mockFetch([{ path: /^\/service\/identity\/roles\?/, body: pageOf([{ id: 'reader', permissions: ['read'] }]) }])
    const result = await listRoles({ start: 50, size: 50 })
    expect(result.data[0].id).toBe('reader')
    expect(calls[0].url).toContain('start=50')
  })

  it('getRole 请求单个角色', async () => {
    const calls = mockFetch([{ path: '/service/identity/roles/reader', body: { id: 'reader' } }])
    expect((await getRole('reader')).id).toBe('reader')
    expect(calls[0].method).toBe('GET')
  })

  it('createRole 提交角色权限', async () => {
    const calls = mockFetch([{ path: '/service/identity/roles', method: 'POST', status: 201, body: { id: 'reader' } }])
    await createRole({ id: 'reader', name: '读取者', permissions: ['read'] })
    expect(calls[0].body).toEqual({ id: 'reader', name: '读取者', permissions: ['read'] })
  })

  it('updateRole 全量提交名称与权限', async () => {
    const calls = mockFetch([{ path: '/service/identity/roles/reader', method: 'PUT', body: { id: 'reader' } }])
    await updateRole('reader', { name: '读取者', permissions: ['read', 'write'] })
    expect(calls[0].body).toEqual({ name: '读取者', permissions: ['read', 'write'] })
  })

  it('deleteRole 使用 DELETE', async () => {
    const calls = mockFetch([{ path: '/service/identity/roles/reader', method: 'DELETE', status: 204 }])
    await expect(deleteRole('reader')).resolves.toBeUndefined()
    expect(calls[0].method).toBe('DELETE')
  })

  it('将服务端错误体映射为 ApiError', async () => {
    mockFetch([
      {
        path: '/service/identity/roles/admin',
        method: 'DELETE',
        status: 403,
        body: { code: 'REST-0403', message: '内置角色不可删除', exception: 'builtin role' },
      },
    ])
    await expect(deleteRole('admin')).rejects.toMatchObject({ status: 403, code: 'REST-0403', message: '内置角色不可删除', exception: 'builtin role' })
    await expect(deleteRole('admin')).rejects.toBeInstanceOf(ApiError)
  })

  it('将重复键错误映射为 ApiError', async () => {
    mockFetch([{ path: '/service/identity/users/alice/info', method: 'POST', status: 409, body: { message: "User info with key 'dept' already exists for this user." } }])
    await expect(createUserInfo('alice', 'dept', '研发')).rejects.toMatchObject({ status: 409, message: "User info with key 'dept' already exists for this user." })
  })
})
