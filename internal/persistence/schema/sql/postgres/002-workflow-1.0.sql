-- workflow 二次封装 Schema 1.0（PostgreSQL）
-- 依据：docs/006-workflow二次封装/003-表结构.md（21 张保留表 + workflow_business_system）。
-- 约定：id varchar(64) 主键（Go idgen 字符串 ID）；权重/比例 numeric(10,2)；
--       时间 timestamp；0/1 标记 smallint；vaild_from/vaild_to 保留 Java 拼写（兼容协议）；
--       condition 为保留字列，使用双引号转义；公共列：creator/creator_name/create_time/updater/update_time/tenant_id；
--       workflow_document/workflow_remind/workflow_remind_target 另有 deleted（逻辑删除，手工过滤）。
-- 引擎 ACT_* 表见 001-flowable-8.1.sql，二者无外键关联，按文件名序执行。

-- ============ 新增：业务系统注册 ============

create table workflow_business_system (
    id varchar(64),
    system_symbol varchar(64),
    system_name varchar(128),
    task_call_addr varchar(500),
    execution_call_addr varchar(500),
    secret varchar(128),
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create unique index uk_wf_biz_system_symbol on workflow_business_system(system_symbol);

-- ============ 配置/定义侧 ============

create table workflow_model (
    id varchar(64),
    resource_id varchar(64),
    model_name varchar(128),
    model_code varchar(64),
    tips varchar(255),
    target varchar(255),
    roles text,
    menu_icon varchar(255),
    remark varchar(500),
    sort int,
    parent_id varchar(64),
    enable smallint default 1,
    return_sort smallint default 0,
    is_leaf smallint default 0,
    process_definition_key varchar(64),
    visible smallint default 1,
    model_id varchar(64),
    last_deploy_time timestamp,
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create unique index uk_wf_model_code_tenant on workflow_model(model_code, tenant_id);
create index idx_wf_model_parent on workflow_model(parent_id);

create table workflow_step (
    id varchar(64),
    name varchar(128),
    step_code varchar(64),
    description varchar(500),
    value varchar(500),
    account_name varchar(128),
    source int,
    model_id varchar(64),
    resource_id varchar(64),
    sort double precision,
    agree varchar(500),
    reject varchar(500),
    remark varchar(500),
    fix text,
    url varchar(500),
    visible smallint default 1,
    task_type varchar(32),
    edit_info text,
    operate_auth text,
    allow_return smallint default 0,
    back_step_codes varchar(1000),
    back_step_names varchar(1000),
    auto_complete smallint default 0,
    auto_complete_start_time timestamp,
    auto_complete_end_time timestamp,
    time_limit double precision,
    time_limit_unit varchar(16),
    remind_time_limit double precision,
    remind_time_limit_unit varchar(16),
    channel_type_name varchar(255),
    channel_type_name_cn varchar(255),
    return_approval_step int,
    countersign_percent_return smallint default 0,
    return_percent numeric(10,2),
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create index idx_wf_step_model on workflow_step(model_id);

create table workflow_his_step (
    id varchar(64),
    step_id varchar(64),
    step_name varchar(128),
    step_code varchar(64),
    account_name varchar(128),
    model_id varchar(64),
    sort double precision,
    agree varchar(500),
    reject varchar(500),
    remark varchar(500),
    url varchar(500),
    visible smallint default 1,
    task_type varchar(32),
    status smallint,
    step_creator varchar(64),
    step_create_time timestamp,
    step_updater varchar(64),
    step_update_time timestamp,
    deployment_id varchar(64),
    edit_info text,
    operate_auth text,
    process_instance_id varchar(64),
    description varchar(500),
    value varchar(500),
    fix text,
    resource_id varchar(64),
    source int,
    allow_return smallint default 0,
    back_step_codes varchar(1000),
    back_step_names varchar(1000),
    auto_complete smallint default 0,
    auto_complete_start_time timestamp,
    auto_complete_end_time timestamp,
    time_limit double precision,
    time_limit_unit varchar(16),
    remind_time_limit double precision,
    remind_time_limit_unit varchar(16),
    channel_type_name varchar(255),
    channel_type_name_cn varchar(255),
    return_approval_step int,
    countersign_percent_return smallint default 0,
    return_percent numeric(10,2),
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create index idx_wf_his_step_deployment on workflow_his_step(deployment_id);
create index idx_wf_his_step_model on workflow_his_step(model_id);

create table workflow_template (
    id varchar(64),
    name varchar(128),
    layout text,
    flow_xml text,
    is_visible smallint default 1,
    is_default smallint default 0,
    source int,
    category_id varchar(64),
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create table workflow_act_extend (
    id varchar(64),
    activity_id varchar(64),
    activity_name varchar(128),
    process_definition_key varchar(64),
    owner_type varchar(16),
    owner_value varchar(64),
    step_id varchar(64),
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create index idx_wf_act_extend_step on workflow_act_extend(step_id);
create index idx_wf_act_extend_def_act on workflow_act_extend(process_definition_key, activity_id);

create table workflow_countersign_weight (
    id varchar(64),
    activity_id varchar(64),
    process_definition_key varchar(64),
    agree_weight numeric(10,2),
    reject_weight numeric(10,2),
    identity_id varchar(64),
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create index idx_wf_csw_def_act on workflow_countersign_weight(process_definition_key, activity_id);

-- ============ 运行时"下一步"机制（预生成-消耗）============

create table workflow_next_activity (
    id varchar(64),
    next_activity_id varchar(64),
    group_id varchar(64),
    identity_id varchar(64),
    send_identity_id varchar(64),
    send_user_name varchar(128),
    process_instance_id varchar(64),
    agree_weight numeric(10,2),
    reject_weight numeric(10,2),
    used smallint default 0,
    pre_task_id varchar(64),
    task_id varchar(64),
    version int default 0,
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create index idx_wf_next_proc_act_used on workflow_next_activity(process_instance_id, next_activity_id, used);
create index idx_wf_next_task on workflow_next_activity(task_id);
create index idx_wf_next_pre_task on workflow_next_activity(pre_task_id);

create table workflow_his_next_activity (
    id varchar(64),
    next_activity_id varchar(64),
    group_id varchar(64),
    identity_id varchar(64),
    send_identity_id varchar(64),
    send_user_name varchar(128),
    process_instance_id varchar(64),
    agree_weight numeric(10,2),
    reject_weight numeric(10,2),
    used smallint default 0,
    pre_task_id varchar(64),
    task_id varchar(64),
    version int default 0,
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create index idx_wf_hnext_proc_act_used on workflow_his_next_activity(process_instance_id, next_activity_id, used);
create index idx_wf_hnext_task on workflow_his_next_activity(task_id);
create index idx_wf_hnext_pre_task on workflow_his_next_activity(pre_task_id);
create index idx_wf_hnext_proc_task_ver on workflow_his_next_activity(process_instance_id, task_id, version);

-- ============ 待办/已办扩展（引擎表 1:1）============

create table workflow_task_message_extend (
    id varchar(64),
    task_id varchar(64),
    process_instance_id varchar(64),
    title varchar(255),
    message text,
    business_id varchar(64),
    target_url varchar(500),
    task_step_name varchar(128),
    task_step_code varchar(64),
    flow_category_code varchar(64),
    flow_category_name varchar(128),
    owner_id varchar(64),
    owner_name varchar(128),
    owner_group varchar(128),
    owner_group_id varchar(64),
    owner_group_paths varchar(1000),
    owner_group_path_ids varchar(1000),
    document_owner_id varchar(64),
    document_owner_name varchar(128),
    document_group_id varchar(64),
    document_group varchar(128),
    document_group_paths varchar(1000),
    document_group_path_ids varchar(1000),
    sender_id varchar(64),
    sender varchar(128),
    sender_step_name varchar(128),
    sender_step_code varchar(64),
    consigner_id varchar(64),
    consigner varchar(128),
    is_read smallint default 0,
    read_time timestamp,
    plan_finish_time timestamp,
    task_receipt_time timestamp,
    task_start_time timestamp,
    is_delay smallint default 0,
    delay_time timestamp,
    message_group_id varchar(64),
    extend_s1 varchar(255),
    extend_s2 varchar(255),
    extend_s3 varchar(255),
    extend_s4 varchar(255),
    extend_s5 varchar(255),
    extend_s6 varchar(255),
    extend_s7 varchar(255),
    extend_s8 varchar(255),
    extend_i1 bigint,
    extend_i2 bigint,
    extend_i3 bigint,
    extend_f1 double precision,
    extend_f2 double precision,
    extend_f3 double precision,
    extend_d1 timestamp,
    extend_d2 timestamp,
    extend_d3 timestamp,
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create unique index uk_wf_message_task on workflow_task_message_extend(task_id);
create index idx_wf_message_proc on workflow_task_message_extend(process_instance_id);
create index idx_wf_message_business on workflow_task_message_extend(business_id, tenant_id);
create index idx_wf_message_owner on workflow_task_message_extend(owner_id);

create table workflow_task_history_extend (
    id varchar(64),
    task_id varchar(64),
    process_instance_id varchar(64),
    title varchar(255),
    message text,
    business_id varchar(64),
    target_url varchar(500),
    task_step_name varchar(128),
    task_step_code varchar(64),
    flow_category_code varchar(64),
    flow_category_name varchar(128),
    owner_id varchar(64),
    owner_name varchar(128),
    owner_group varchar(128),
    owner_group_id varchar(64),
    owner_group_paths varchar(1000),
    owner_group_path_ids varchar(1000),
    document_owner_id varchar(64),
    document_owner_name varchar(128),
    document_group_id varchar(64),
    document_group varchar(128),
    document_group_paths varchar(1000),
    document_group_path_ids varchar(1000),
    sender_id varchar(64),
    sender varchar(128),
    sender_step_name varchar(128),
    sender_step_code varchar(64),
    consigner_id varchar(64),
    consigner varchar(128),
    is_read smallint default 0,
    read_time timestamp,
    plan_finish_time timestamp,
    task_receipt_time timestamp,
    task_start_time timestamp,
    is_delay smallint default 0,
    delay_time timestamp,
    message_group_id varchar(64),
    task_finish_time timestamp,
    extend_s1 varchar(255),
    extend_s2 varchar(255),
    extend_s3 varchar(255),
    extend_s4 varchar(255),
    extend_s5 varchar(255),
    extend_s6 varchar(255),
    extend_s7 varchar(255),
    extend_s8 varchar(255),
    extend_i1 bigint,
    extend_i2 bigint,
    extend_i3 bigint,
    extend_f1 double precision,
    extend_f2 double precision,
    extend_f3 double precision,
    extend_d1 timestamp,
    extend_d2 timestamp,
    extend_d3 timestamp,
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create unique index uk_wf_history_task on workflow_task_history_extend(task_id);
create index idx_wf_history_proc on workflow_task_history_extend(process_instance_id);

create table workflow_delegate_task (
    id varchar(64),
    task_id varchar(64),
    process_instance_id varchar(64),
    title varchar(255),
    message text,
    business_id varchar(64),
    target_url varchar(500),
    task_step_name varchar(128),
    task_step_code varchar(64),
    flow_category_code varchar(64),
    flow_category_name varchar(128),
    owner_id varchar(64),
    owner_name varchar(128),
    owner_group varchar(128),
    owner_group_id varchar(64),
    owner_group_paths varchar(1000),
    owner_group_path_ids varchar(1000),
    document_owner_id varchar(64),
    document_owner_name varchar(128),
    document_group_id varchar(64),
    document_group varchar(128),
    document_group_paths varchar(1000),
    document_group_path_ids varchar(1000),
    sender_id varchar(64),
    sender varchar(128),
    sender_step_name varchar(128),
    sender_step_code varchar(64),
    consigner_id varchar(64),
    consigner varchar(128),
    is_read smallint default 0,
    read_time timestamp,
    plan_finish_time timestamp,
    task_receipt_time timestamp,
    task_start_time timestamp,
    is_delay smallint default 0,
    delay_time timestamp,
    message_group_id varchar(64),
    entrust_id varchar(64),
    entrust_name varchar(128),
    extend_s1 varchar(255),
    extend_s2 varchar(255),
    extend_s3 varchar(255),
    extend_s4 varchar(255),
    extend_s5 varchar(255),
    extend_s6 varchar(255),
    extend_s7 varchar(255),
    extend_s8 varchar(255),
    extend_i1 bigint,
    extend_i2 bigint,
    extend_i3 bigint,
    extend_f1 double precision,
    extend_f2 double precision,
    extend_f3 double precision,
    extend_d1 timestamp,
    extend_d2 timestamp,
    extend_d3 timestamp,
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create index idx_wf_dtask_task on workflow_delegate_task(task_id);
create index idx_wf_dtask_proc on workflow_delegate_task(process_instance_id);

-- ============ 留痕/影子 ============

create table workflow_document (
    id varchar(64),
    business_id varchar(64),
    create_group_path_ids varchar(1000),
    create_group_paths varchar(1000),
    create_group_id varchar(64),
    create_group varchar(128),
    updater_name varchar(128),
    deleted smallint default 0,
    document_url varchar(500),
    document_id varchar(64),
    document_table varchar(128),
    flow_title varchar(255),
    flow_task_id varchar(64),
    flow_instance_id varchar(64),
    flow_category_id varchar(64),
    flow_category varchar(128),
    flow_action_id varchar(64),
    flow_action varchar(128),
    flow_status varchar(64),
    flow_current_step_name varchar(255),
    flow_current_persons varchar(1000),
    flow_start_time timestamp,
    flow_finish_time timestamp,
    flow_termination_time timestamp,
    extend_s1 varchar(255),
    extend_s2 varchar(255),
    extend_s3 varchar(255),
    extend_s4 varchar(255),
    extend_s5 varchar(255),
    extend_s6 varchar(255),
    extend_s7 varchar(255),
    extend_s8 varchar(255),
    extend_i1 bigint,
    extend_i2 bigint,
    extend_i3 bigint,
    extend_f1 double precision,
    extend_f2 double precision,
    extend_f3 double precision,
    extend_d1 timestamp,
    extend_d2 timestamp,
    extend_d3 timestamp,
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create unique index uk_wf_document_business on workflow_document(business_id, tenant_id);
create index idx_wf_document_instance on workflow_document(flow_instance_id);

create table workflow_operation_record (
    id varchar(64),
    business_id varchar(64),
    task_id varchar(64),
    process_instance_id varchar(64),
    message text,
    is_agree smallint,
    task_step_code varchar(64),
    task_step_name varchar(128),
    flow_category_code varchar(64),
    flow_category_name varchar(128),
    owner_id varchar(64),
    owner_name varchar(128),
    owner_group varchar(128),
    owner_group_id varchar(64),
    owner_group_paths varchar(1000),
    owner_group_path_ids varchar(1000),
    deal_time timestamp,
    task_receipt_time timestamp,
    task_start_time timestamp,
    task_finish_time timestamp,
    type varchar(32),
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create index idx_wf_record_task on workflow_operation_record(task_id);
create index idx_wf_record_business on workflow_operation_record(business_id);

create table workflow_review_task (
    id varchar(64),
    start_time timestamp,
    task_id varchar(64),
    task_name varchar(128),
    process_instance_id varchar(64),
    process_definition_id varchar(64),
    task_key varchar(64),
    business_key varchar(64),
    process_name varchar(128),
    process_definition_key varchar(64),
    process_definition_name varchar(128),
    sender varchar(64),
    sender_name varchar(128),
    reviewer varchar(64),
    review_time timestamp,
    remark varchar(500),
    opinion text,
    status smallint default 0,
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create index idx_wf_review_proc on workflow_review_task(process_instance_id);
create index idx_wf_review_reviewer on workflow_review_task(reviewer);

create table workflow_task_exception (
    id varchar(64),
    task_id varchar(64),
    process_instance_id varchar(64),
    process_instance_name varchar(255),
    class_name varchar(255),
    method_name varchar(128),
    message text,
    params text,
    exception_type varchar(64),
    exception_class varchar(255),
    process_definition_id varchar(64),
    process_definition_key varchar(64),
    process_definition_name varchar(128),
    start_user_id varchar(64),
    handle_status int default 0,
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create index idx_wf_exception_status on workflow_task_exception(handle_status);
create index idx_wf_exception_proc on workflow_task_exception(process_instance_id);

create table workflow_complete_to_activity (
    id varchar(64),
    flow_id varchar(64),
    source_flow_element_id varchar(64),
    target_flow_element_id varchar(64),
    task_id varchar(64),
    process_instance_id varchar(64),
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create index idx_wf_complete_proc on workflow_complete_to_activity(process_instance_id);

-- ============ 委托体系（新三表）============

create table workflow_task_delegate (
    id varchar(64),
    consign_type varchar(32),
    owner_id varchar(64),
    owner_name varchar(128),
    source varchar(64),
    source_code varchar(64),
    source_name varchar(128),
    type varchar(32),
    "condition" varchar(500),
    vaild_from timestamp,
    vaild_to timestamp,
    enable smallint default 1,
    retire smallint default 0,
    remark varchar(500),
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create index idx_wf_tdelegate_owner on workflow_task_delegate(owner_id);
create index idx_wf_tdelegate_source on workflow_task_delegate(source_code);

create table workflow_task_consigners (
    id varchar(64),
    delegate_id varchar(64),
    consigner_id varchar(64),
    consigner_name varchar(128),
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create index idx_wf_tconsigners_delegate on workflow_task_consigners(delegate_id);

create table workflow_task_consign_obj (
    id varchar(64),
    delegate_id varchar(64),
    owner_type varchar(32),
    owner_id varchar(64),
    owner_name varchar(128),
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create index idx_wf_tconsignobj_delegate on workflow_task_consign_obj(delegate_id);

-- ============ 提醒体系 ============

create table workflow_remind (
    id varchar(64),
    pid varchar(64),
    step_ids varchar(1000),
    enable_pre smallint default 0,
    enable_post smallint default 0,
    sent smallint default 0,
    deleted smallint default 0,
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create index idx_wf_remind_pid on workflow_remind(pid);

create table workflow_remind_target (
    id varchar(64),
    pid varchar(64),
    executor_id varchar(64),
    enable smallint default 1,
    name varchar(128),
    target_type smallint,
    type smallint,
    retry smallint,
    retry_duration int,
    retry_times int,
    retry_count int,
    retry_time_unit varchar(16),
    ahead int,
    delay int,
    by_plan_complete_time smallint default 0,
    removable smallint default 0,
    sent smallint default 0,
    deleted smallint default 0,
    creator varchar(64),
    creator_name varchar(128),
    create_time timestamp,
    updater varchar(64),
    update_time timestamp,
    tenant_id varchar(64),
    primary key (id)
);

create index idx_wf_remind_target_pid on workflow_remind_target(pid);
