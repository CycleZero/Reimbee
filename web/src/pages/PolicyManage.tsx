import {
  Card, Table, Button, Space, Modal, Form, Input, Select,
  Tag, Skeleton, App, Tooltip, Tabs, Descriptions, Empty, InputNumber,
} from 'antd';
import {
  PlusOutlined, ReloadOutlined, SearchOutlined,
  EyeOutlined, EditOutlined, DeleteOutlined,
  DatabaseOutlined, CheckCircleOutlined, CloseCircleOutlined,
} from '@ant-design/icons';
import { useEffect, useState, useCallback } from 'react';
import {
  listPolicies, getPolicy, createPolicy, updatePolicy, deletePolicy,
  getKBStatus, searchPolicies,
} from '@/api';
import type {
  PolicyDocument, PolicyDocumentDetail, KnowledgeBaseStatus, SearchTestResult,
} from '@/types/models';

const { TextArea } = Input;

export default function PolicyManage() {
  const { message, modal } = App.useApp();
  const [loading, setLoading] = useState(true);
  const [docs, setDocs] = useState<PolicyDocument[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState<KnowledgeBaseStatus | null>(null);

  // 创建弹窗
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm] = Form.useForm();
  const [creating, setCreating] = useState(false);

  // 详情抽屉
  const [detailOpen, setDetailOpen] = useState(false);
  const [detail, setDetail] = useState<PolicyDocumentDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  // 编辑弹窗
  const [editOpen, setEditOpen] = useState(false);
  const [editForm] = Form.useForm();
  const [editing, setEditing] = useState(false);
  const [editId, setEditId] = useState<number>(0);

  // 搜索测试
  const [searchQuery, setSearchQuery] = useState('');
  const [searchResult, setSearchResult] = useState<SearchTestResult | null>(null);
  const [searching, setSearching] = useState(false);

  const fetchList = useCallback(async () => {
    setLoading(true);
    try {
      const res = await listPolicies({ page, page_size: 10 });
      setDocs(res.list);
      setTotal(res.total);
    } catch {
      message.error('加载文档列表失败');
    } finally {
      setLoading(false);
    }
  }, [page, message]);

  const fetchStatus = useCallback(async () => {
    try {
      setStatus(await getKBStatus());
    } catch { /* ignore */ }
  }, []);

  useEffect(() => { fetchList(); fetchStatus(); }, [fetchList, fetchStatus]);

  const handleCreate = async () => {
    try {
      const values = await createForm.validateFields();
      setCreating(true);
      await createPolicy(values);
      message.success('文档创建成功');
      setCreateOpen(false);
      createForm.resetFields();
      fetchList();
      fetchStatus();
    } catch {
      // validation error handled by form
    } finally {
      setCreating(false);
    }
  };

  const handleView = async (id: number) => {
    setDetailLoading(true);
    setDetailOpen(true);
    try {
      setDetail(await getPolicy(id));
    } catch {
      message.error('加载文档详情失败');
    } finally {
      setDetailLoading(false);
    }
  };

  const handleEdit = async (id: number) => {
    try {
      const doc = await getPolicy(id);
      setEditId(id);
      editForm.setFieldsValue(doc);
      setEditOpen(true);
    } catch {
      message.error('加载文档失败');
    }
  };

  const handleEditSubmit = async () => {
    try {
      const values = await editForm.validateFields();
      setEditing(true);
      await updatePolicy(editId, values);
      message.success('更新成功');
      setEditOpen(false);
      fetchList();
      fetchStatus();
    } catch {
      // validation error
    } finally {
      setEditing(false);
    }
  };

  const handleDelete = (id: number, title: string) => {
    modal.confirm({
      title: '确认删除',
      content: `确定要删除文档「${title}」吗？删除后不可恢复。`,
      okText: '删除',
      okType: 'danger',
      onOk: async () => {
        await deletePolicy(id);
        message.success('已删除');
        fetchList();
        fetchStatus();
      },
    });
  };

  const handleSearch = async () => {
    if (!searchQuery.trim()) return;
    setSearching(true);
    try {
      setSearchResult(await searchPolicies(searchQuery, 5));
    } catch {
      message.error('搜索失败');
    } finally {
      setSearching(false);
    }
  };

  const columns = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '标题', dataIndex: 'title', ellipsis: true },
    { title: '版本', dataIndex: 'version', width: 80 },
    { title: '状态', dataIndex: 'status', width: 80, render: (s: string) =>
      <Tag color={s === 'active' ? 'green' : 'default'}>{s === 'active' ? '启用' : '归档'}</Tag> },
    { title: '分块数', dataIndex: 'chunk_count', width: 80 },
    { title: '更新时间', dataIndex: 'updated_at', width: 170 },
    {
      title: '操作', width: 200, render: (_: unknown, r: PolicyDocument) => (
        <Space>
          <Button size="small" icon={<EyeOutlined />} onClick={() => handleView(r.id)}>详情</Button>
          <Button size="small" icon={<EditOutlined />} onClick={() => handleEdit(r.id)}>编辑</Button>
          <Button size="small" danger icon={<DeleteOutlined />} onClick={() => handleDelete(r.id, r.title)}>删除</Button>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <h2 style={{ margin: 0 }}>📚 知识库管理</h2>
        <Space>
          {status && (
            <Tooltip title={`搜索模式: ${status.search_mode === 'vector' ? '向量搜索' : '关键词匹配'}`}>
              <Tag icon={status.healthy ? <CheckCircleOutlined /> : <CloseCircleOutlined />}
                   color={status.healthy ? 'green' : 'red'}>
                {status.search_mode === 'vector' ? status.vector_store : '关键词'} · {status.document_count}文档 · {status.chunk_count}分块
              </Tag>
            </Tooltip>
          )}
          <Button icon={<ReloadOutlined />} onClick={() => { fetchList(); fetchStatus(); }}>刷新</Button>
        </Space>
      </div>

      <Tabs defaultActiveKey="docs" items={[
        {
          key: 'docs', label: '文档列表', children: (
            <>
              <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between' }}>
                <Space.Compact style={{ width: 400 }}>
                  <Input placeholder="输入关键词搜索知识库..." value={searchQuery}
                         onChange={e => setSearchQuery(e.target.value)}
                         onPressEnter={handleSearch} />
                  <Button icon={<SearchOutlined />} loading={searching} onClick={handleSearch}>搜索</Button>
                </Space.Compact>
                <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>新建文档</Button>
              </div>
              {searchResult && (
                <Card size="small" title={`搜索结果: ${searchResult.query} (${searchResult.mode})`} style={{ marginBottom: 16 }}
                      extra={<Button size="small" onClick={() => setSearchResult(null)}>清除</Button>}>
                  {searchResult.chunks.length === 0 ? <Empty description="无匹配结果" /> : (
                    searchResult.chunks.map((c, i) => (
                      <Card key={i} size="small" style={{ marginBottom: 8 }}
                            title={`${c.document_title} · 分块${c.chunk_index}`}>
                        <p style={{ whiteSpace: 'pre-wrap', margin: 0 }}>{c.content}</p>
                      </Card>
                    ))
                  )}
                </Card>
              )}
              <Table rowKey="id" columns={columns} dataSource={docs} loading={loading}
                     pagination={{ current: page, total, pageSize: 10, onChange: setPage }} />
            </>
          ),
        },
        {
          key: 'status', label: '知识库状态', children: (
            <Card loading={!status}>
              {status && (
                <Descriptions column={2} bordered size="small">
                  <Descriptions.Item label="文档数">{status.document_count}</Descriptions.Item>
                  <Descriptions.Item label="分块数">{status.chunk_count}</Descriptions.Item>
                  <Descriptions.Item label="搜索模式">{status.search_mode === 'vector' ? '向量语义搜索' : '关键词匹配'}</Descriptions.Item>
                  <Descriptions.Item label="健康状态">
                    <Tag color={status.healthy ? 'green' : 'red'}>{status.healthy ? '正常' : '异常'}</Tag>
                  </Descriptions.Item>
                  {status.embedder_model && <Descriptions.Item label="嵌入模型">{status.embedder_model}</Descriptions.Item>}
                  {status.vector_store && <Descriptions.Item label="向量数据库">{status.vector_store}</Descriptions.Item>}
                </Descriptions>
              )}
            </Card>
          ),
        },
      ]} />

      {/* 创建弹窗 */}
      <Modal title="新建政策文档" open={createOpen} onOk={handleCreate} onCancel={() => setCreateOpen(false)}
             confirmLoading={creating} width={640} destroyOnClose>
        <Form form={createForm} layout="vertical">
          <Form.Item name="title" label="标题" rules={[{ required: true, message: '请输入标题' }]}>
            <Input placeholder="如：差旅费报销管理办法" />
          </Form.Item>
          <Form.Item name="content" label="内容" rules={[{ required: true, message: '请输入内容' }]}>
            <TextArea rows={10} placeholder="政策文档全文..." />
          </Form.Item>
          <Form.Item name="version" label="版本" initialValue="v1">
            <Input placeholder="v1" />
          </Form.Item>
          <Form.Item name="effective_date" label="生效日期">
            <Input placeholder="2026-01-01" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 详情抽屉 */}
      <Modal title={detail?.title} open={detailOpen} onCancel={() => setDetailOpen(false)} footer={null}
             width={720} destroyOnClose>
        {detailLoading ? <Skeleton active /> : detail && (
          <div>
            <Descriptions column={2} size="small" style={{ marginBottom: 16 }}>
              <Descriptions.Item label="版本">{detail.version}</Descriptions.Item>
              <Descriptions.Item label="状态"><Tag color={detail.status === 'active' ? 'green' : 'default'}>{detail.status}</Tag></Descriptions.Item>
              <Descriptions.Item label="生效日期">{detail.effective_date || '-'}</Descriptions.Item>
              <Descriptions.Item label="分块数">{detail.chunk_count}</Descriptions.Item>
            </Descriptions>
            <h4>全文</h4>
            <pre style={{ whiteSpace: 'pre-wrap', background: '#f5f5f5', padding: 12, borderRadius: 8, maxHeight: 200, overflow: 'auto' }}>{detail.content}</pre>
            <h4 style={{ marginTop: 16 }}>分块列表 ({detail.chunks.length})</h4>
            {detail.chunks.map(c => (
              <Card key={c.id} size="small" style={{ marginBottom: 8 }} title={`分块 #${c.chunk_index}`}>
                <p style={{ whiteSpace: 'pre-wrap', margin: 0, fontSize: 13 }}>{c.content}</p>
              </Card>
            ))}
          </div>
        )}
      </Modal>

      {/* 编辑弹窗 */}
      <Modal title="编辑政策文档" open={editOpen} onOk={handleEditSubmit} onCancel={() => setEditOpen(false)}
             confirmLoading={editing} width={640} destroyOnClose>
        <Form form={editForm} layout="vertical">
          <Form.Item name="title" label="标题" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="content" label="内容" rules={[{ required: true }]}>
            <TextArea rows={10} />
          </Form.Item>
          <Form.Item name="version" label="版本"><Input /></Form.Item>
          <Form.Item name="effective_date" label="生效日期"><Input /></Form.Item>
          <Form.Item name="status" label="状态">
            <Select options={[{ label: '启用', value: 'active' }, { label: '归档', value: 'archived' }]} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
