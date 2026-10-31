import { useEffect, useRef } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { errorMessage } from '@/lib/api';
import { clearEmailTokenHash, emailTokenFromHash } from '@/lib/email-verification';
import { useSession } from '@/lib/session';
import { passwordRequirementsText } from '@/lib/password-policy';
import { UserAvatar } from '@/components/WorkbenchShell';
import { Confirm, UnsavedGuard } from '@/components/management/shared';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Skeleton } from '@/components/ui/skeleton';
import { useProfileEditor } from './use-profile-editor';

function FieldErrors({ name, fields }: { name: string; fields: Record<string, string[]> }) {
  return fields[name]?.length ? <ul id={`profile-${name}-errors`} className="field-errors" aria-live="polite">{fields[name].map((message, index) => <li key={index}>{message}</li>)}</ul> : null;
}

export function ProfilePage() {
  const { context, sessionId } = useSession();
  if (!context || !sessionId) return null;
  return <ProfileEditor key={`${context.user.id}:${sessionId}`} userID={context.user.id} sessionID={sessionId} />;
}

function ProfileEditor({ userID, sessionID }: { userID: number; sessionID: string }) {
  const { context } = useSession();
  const location = useLocation();
  const editor = useProfileEditor(userID, sessionID);
  const { user, fields, activeEditor } = editor;
  const disabled = editor.busy || editor.reading || editor.suspended;
  const autoToken = useRef<string | null>(null);
  useEffect(() => {
    const token = emailTokenFromHash(location.hash);
    if (!token || !user || autoToken.current === token) return;
    autoToken.current = token;
    clearEmailTokenHash();
    editor.setEmailToken(token);
    void editor.confirmEmail(token);
  }, [location.hash, user, editor]);
  useEffect(() => {
    if (!editor.busy && Object.keys(fields).length) document.getElementById(`profile-${Object.keys(fields)[0]}`)?.focus();
  }, [fields, editor.busy]);

  return <section className="profile-page">
    <UnsavedGuard dirty={editor.dirty} busy={editor.busy} />
    <span className="eyebrow">当前账号</span><h1>个人资料</h1>
    <p className="intro">资料来自当前会话的真实用户信息。角色与权限以实际授权上下文为准。</p>
    {editor.outcome && <Alert variant={editor.outcome.kind === 'saved' ? 'default' : 'destructive'} role={editor.outcome.kind === 'saved' ? 'status' : 'alert'} className="profile-feedback"><AlertDescription>
      <p>{editor.outcome.message}</p>
      {editor.outcome.kind === 'unknown' && editor.outcome.password && <Button variant="outline" asChild><Link to="/login" state={{ relogin: true, from: '/profile' }}>重新登录核对</Link></Button>}
    </AlertDescription></Alert>}
    {Boolean(editor.displayError || editor.readError) && <Alert variant="destructive" className="profile-feedback"><AlertDescription>
      <p>{editor.confirmedSave ? '已保存，显示更新未完成。' : '资料或头像显示读取未完成。'}{editor.readError ? errorMessage(editor.readError) : '请重新读取受控显示。'}</p>
      <Button variant="outline" disabled={disabled} onClick={() => void editor.readDisplay()}>重新读取显示</Button>
    </AlertDescription></Alert>}
    {editor.outcome?.kind === 'unknown' && !editor.outcome.password && !editor.displayError && <div className="detail-actions profile-feedback"><Button variant="outline" disabled={disabled} onClick={() => void editor.readDisplay()}>重新读取已保存资料核对</Button><span className="hint">只读取，不重新提交，也不丢弃当前草稿。</span></div>}
    {!user ? editor.readError ? <Button variant="outline" disabled={disabled} onClick={() => void editor.readDisplay()}>重新加载个人资料</Button> : <div role="status" aria-label="正在加载个人资料"><Skeleton className="profile-skeleton" /><span className="hint">正在加载个人资料…</span></div> : <>
      <Card className="detail-hero">
        <div className="profile-identity">
          <UserAvatar key={`${userID}:${sessionID}`} id={user.id} name={user.nickname || user.username} revision={editor.userRevision} onLoad={() => editor.imageResult(false)} onError={() => editor.imageResult(true)} />
          <div><h2>{user.nickname || user.username}</h2><code>{user.username}</code></div>
          <Badge variant={user.status === 1 ? 'default' : 'destructive'}>{user.status === 1 ? '已启用' : '已停用'}</Badge>
          <Button variant="outline" disabled={disabled} aria-expanded={activeEditor === 'avatar'} aria-controls="profile-avatar-editor" onClick={() => editor.openEditor('avatar')}>编辑头像</Button>
        </div>
        {activeEditor === 'avatar' && <div id="profile-avatar-editor" className="profile-inline-editor">
          <h3>头像</h3><p className="hint">仅支持本地静态 JPEG、PNG、WebP。选择只作预览，保存后以服务端标准化图片为准。</p>
          {editor.policy.state === 'loading' ? <p className="hint" role="status">正在读取头像上传规则…</p> : editor.policy.state === 'error' ? <Alert variant="destructive"><AlertDescription><p>头像上传规则不可用，暂不能选图或上传。{errorMessage(editor.policy.error)}</p><Button variant="outline" disabled={editor.suspended} onClick={() => void editor.readPolicy()}>重试读取上传规则</Button></AlertDescription></Alert> : <p id="profile-file-help" className="hint">当前文件上限：{editor.policy.maxBytes} 字节。服务端仍会校验图片内容、像素和标准化输出。</p>}
          {editor.policy.state === 'ready' && <Button variant="link" disabled={disabled} onClick={() => void editor.readPolicy()}>重新读取上传规则</Button>}
          <div className="profile-field"><label htmlFor="profile-file">选择本地头像</label><Input id="profile-file" type="file" accept="image/jpeg,image/png,image/webp,.jpg,.jpeg,.png,.webp" disabled={disabled || editor.policy.state !== 'ready'} aria-invalid={!!fields.file} aria-describedby={fields.file ? 'profile-file-errors' : editor.policy.state === 'ready' ? 'profile-file-help' : undefined} onChange={event => { editor.chooseFile(event.target.files?.[0]); event.target.value = ''; }} /><FieldErrors name="file" fields={fields} /></div>
          {editor.preview && <figure className="profile-avatar-preview"><img src={editor.preview} alt="待保存的头像预览" /><figcaption>{editor.file?.name}<span className="hint">尚未上传，不影响顶栏头像。</span></figcaption></figure>}
          <div className="detail-actions"><Button disabled={disabled || !editor.file || editor.policy.state !== 'ready'} onClick={() => void editor.submit('avatar')}>{editor.busy ? '正在保存…' : '保存头像'}</Button><Button variant="outline" disabled={disabled} onClick={editor.cancel}>取消头像编辑</Button><Button variant="ghost" disabled={disabled} onClick={() => editor.setResetOpen(true)}>恢复默认头像</Button></div>
        </div>}
      </Card>
      <div className="profile-grid">
        <Card><CardHeader><CardTitle>身份资料</CardTitle></CardHeader><CardContent>
          <dl className="profile-fields"><div><dt>用户 ID</dt><dd><code>{user.id}</code></dd></div><div><dt>用户名</dt><dd>{user.username}</dd></div><div><dt>昵称</dt><dd>{user.nickname || '未设置'}</dd></div></dl>
          {activeEditor === 'nickname' ? <form id="profile-nickname-editor" className="profile-inline-editor" noValidate onSubmit={event => { event.preventDefault(); void editor.submit('nickname'); }}>
            <div className="profile-field"><label htmlFor="profile-nickname">昵称</label><Input id="profile-nickname" name="nickname" autoComplete="nickname" value={editor.nickname} disabled={disabled} aria-invalid={!!fields.nickname} aria-describedby={fields.nickname ? 'profile-nickname-errors profile-nickname-help' : 'profile-nickname-help'} onChange={event => editor.setNickname(event.target.value)} /><p id="profile-nickname-help" className="hint">必填，最多 100 个 Unicode 字符；首尾空白和大小写按原样保存。</p><FieldErrors name="nickname" fields={fields} /></div>
            <div className="detail-actions"><Button type="submit" disabled={disabled}>{editor.busy ? '正在保存…' : '保存昵称'}</Button><Button type="button" variant="outline" disabled={disabled} onClick={editor.cancel}>取消昵称编辑</Button></div>
          </form> : <div className="detail-actions"><Button variant="outline" disabled={disabled} aria-expanded={false} aria-controls="profile-nickname-editor" onClick={() => editor.openEditor('nickname')}>编辑昵称</Button></div>}
        </CardContent></Card>
        <Card><CardHeader><CardTitle>邮箱</CardTitle></CardHeader><CardContent>
          <p>{user.email || '未设置邮箱'}</p>
          {user.email && <Badge variant={user.email_verified ? 'default' : 'secondary'}>{user.email_verified ? '已验证' : '未验证'}</Badge>}
          {user.pending_email && <p className="warning-note">待验证的新邮箱：{user.pending_email}</p>}
          {editor.emailPolicy.state === 'loading' && <p className="hint" role="status">正在读取邮箱验证策略…</p>}
          {editor.emailPolicy.state === 'error' && <Alert variant="destructive"><AlertDescription><p>邮箱验证策略读取失败，暂不能换绑或重发。{errorMessage(editor.emailPolicy.error)}</p><Button variant="outline" disabled={editor.suspended} onClick={() => void editor.readEmailPolicy()}>重试读取策略</Button></AlertDescription></Alert>}
          {editor.emailPolicy.state === 'ready' && !editor.emailPolicy.enabled && <p className="hint">当前部署未启用邮件服务，邮箱换绑与重发暂不可用；如已收到验证码仍可验证，待验证邮箱仍可取消。</p>}
          {activeEditor === 'email' ? <div id="profile-email-editor" className="profile-inline-editor">
            <h3>更换邮箱</h3>
            <p className="hint">{user.email_verified ? '当前邮箱已验证。新邮箱会先进入待验证，验证成功后才替换当前邮箱。' : '当前邮箱未验证。保存后会立即替换当前邮箱，原未验证邮箱不再保留。'}</p>
            <form className="profile-email-form" noValidate onSubmit={event => { event.preventDefault(); void editor.submit('email'); }}>
              <div className="profile-field"><label htmlFor="profile-email">新邮箱</label><Input id="profile-email" name="email" type="email" autoComplete="email" value={editor.emailCandidate} disabled={disabled} aria-invalid={!!fields.email} aria-describedby={fields.email ? 'profile-email-errors' : undefined} onChange={event => editor.setEmailCandidate(event.target.value)} /><FieldErrors name="email" fields={fields} /></div>
              <div className="profile-field"><label htmlFor="profile-current-password">当前密码</label><Input id="profile-current-password" name="current_password" type="password" autoComplete="current-password" value={editor.emailCurrentPassword} disabled={disabled} aria-invalid={!!fields.current_password} aria-describedby={fields.current_password ? 'profile-current-password-errors' : undefined} onChange={event => editor.setEmailCurrentPassword(event.target.value)} /><FieldErrors name="current_password" fields={fields} /></div>
              <div className="detail-actions"><Button type="submit" disabled={disabled || editor.emailPolicy.state !== 'ready' || !editor.emailPolicy.enabled}>{editor.busy ? '正在提交…' : '提交换绑'}</Button><Button type="button" variant="outline" disabled={disabled} onClick={editor.cancel}>取消邮箱编辑</Button></div>
            </form>
            <h3>验证邮箱</h3>
            {user.pending_email || !user.email_verified ? <form className="profile-email-form" noValidate onSubmit={event => { event.preventDefault(); void editor.confirmEmail(editor.emailToken); }}>
              <p className="hint">{user.pending_email ? `验证待验证邮箱：${user.pending_email}` : `验证当前邮箱：${user.email}`}</p>
              <div className="profile-field"><label htmlFor="profile-email-token">验证码</label><Input id="profile-email-token" name="token" autoComplete="one-time-code" value={editor.emailToken} disabled={disabled} aria-invalid={!!fields.token} aria-describedby={fields.token ? 'profile-email-token-errors' : 'profile-email-token-help'} onChange={event => editor.setEmailToken(event.target.value)} /><p id="profile-email-token-help" className="hint">从验证邮件中粘贴验证码；打开邮件链接会自动提交。</p><FieldErrors name="token" fields={fields} /></div>
              <div className="detail-actions"><Button type="submit" disabled={disabled}>{editor.busy ? '正在验证…' : '确认验证'}</Button><Button type="button" variant="outline" disabled={disabled || editor.emailPolicy.state !== 'ready' || !editor.emailPolicy.enabled} onClick={() => void editor.resendEmail()}>重发验证邮件</Button>{user.pending_email && <Button type="button" variant="outline" disabled={disabled} onClick={() => void editor.cancelPendingEmail()}>取消待验证邮箱</Button>}</div>
            </form> : <p className="hint">当前邮箱已验证。</p>}
          </div> : <div className="detail-actions"><Button variant="outline" disabled={disabled} aria-expanded={false} aria-controls="profile-email-editor" onClick={() => editor.openEditor('email')}>编辑邮箱</Button>{user.pending_email && <Button variant="outline" disabled={disabled} onClick={() => editor.openEditor('email')}>验证待验证邮箱</Button>}</div>}
          <p className="hint profile-readonly-note">邮箱和验证状态来自服务器；验证码请从验证邮件中获取，链接会自动提交。</p>
        </CardContent></Card>
        <Card><CardHeader><CardTitle>当前角色</CardTitle></CardHeader><CardContent>{context?.roles.length ? <ul className="profile-role-list">{context.roles.map(role => <li key={role}><code>{role}</code></li>)}</ul> : <p className="hint">尚未分配角色</p>}<p className="hint">角色变更后，旧会话会在后续请求失效。</p></CardContent></Card>
      </div>
      <Card className="profile-password-card"><CardHeader><CardTitle>账号密码</CardTitle></CardHeader><CardContent>
        <p className="hint">修改成功后将退出当前浏览器会话，请使用新密码重新登录。</p>
        {activeEditor === 'password' ? <form id="profile-password-editor" className="profile-password-form" noValidate onSubmit={event => { event.preventDefault(); void editor.submit('password'); }}>
          {(['old_password', 'new_password', 'confirm_password'] as const).map(name => <div className="profile-field" key={name}><label htmlFor={`profile-${name}`}>{name === 'old_password' ? '旧密码' : name === 'new_password' ? '新密码' : '确认新密码'}</label><Input id={`profile-${name}`} name={name} type="password" autoComplete={name === 'old_password' ? 'current-password' : 'new-password'} value={editor.passwords[name]} disabled={disabled} aria-invalid={!!fields[name]} aria-describedby={fields[name] ? `profile-${name}-errors${name === 'new_password' ? ' profile-password-help' : ''}` : name === 'new_password' ? 'profile-password-help' : undefined} onChange={event => editor.setPassword(name, event.target.value)} />{name === 'new_password' && <p id="profile-password-help" className="hint">{passwordRequirementsText}</p>}<FieldErrors name={name} fields={fields} /></div>)}
          <div className="detail-actions"><Button type="submit" disabled={disabled}>{editor.busy ? '正在修改…' : '修改密码并退出'}</Button><Button type="button" variant="outline" disabled={disabled} onClick={editor.cancel}>取消密码编辑</Button></div>
        </form> : <Button variant="outline" disabled={disabled} aria-expanded={false} aria-controls="profile-password-editor" onClick={() => editor.openEditor('password')}>修改密码</Button>}
      </CardContent></Card>
      {(editor.busy || editor.displayRefreshing) && <p className="hint profile-operation-status" role="status">{editor.busy ? '正在提交当前操作，请勿重复提交或切换编辑区。' : '正在重新读取资料与受控头像…'}</p>}
    </>}
    <Confirm open={editor.switchTo !== null} onOpenChange={open => { if (!open) editor.setSwitchTo(null); }} title="放弃未保存的修改？" label="放弃修改并切换" onConfirm={editor.confirmSwitch}>当前编辑尚未保存。取消将保留全部输入和待保存头像预览。</Confirm>
    <Confirm open={editor.resetOpen} onOpenChange={editor.setResetOpen} title="恢复默认头像？" label="确认恢复默认头像" busy={editor.busy} onConfirm={() => void editor.submit('reset')}>确认后立即恢复默认头像；成功后才会丢弃当前待保存预览。取消不会更改头像或草稿。</Confirm>
  </section>;
}
