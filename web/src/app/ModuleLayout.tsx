import { useEffect, useState } from 'react';
import { Button } from 'antd';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import { isDemo } from '../shared/api/client';
import { accountingModule, activeModuleTab, moduleTabs } from './module-navigation';
import styles from './App.module.css';

export function ModuleLayout() {
  const { pathname, search } = useLocation();
  const navigate = useNavigate();
  const module = accountingModule(pathname);
  const active = activeModuleTab(pathname, search);
  const [targets, setTargets] = useState<Record<string, string>>({});
  const key = `${module?.path}/${active}`;
  useEffect(() => {
    setTargets(previous => previous[key] === pathname + search ? previous : { ...previous, [key]: pathname + search });
  }, [key, pathname, search]);
  return <>
    {!isDemo && <nav className={styles.moduleNavigation} aria-label="Điều hướng phân hệ">
      {moduleTabs(pathname).map(tab => <Button key={tab.key} type="text" aria-current={active === tab.key ? 'page' : undefined} className={active === tab.key ? styles.moduleActive : undefined} onClick={() => navigate(targets[`${module?.path}/${tab.key}`] ?? tab.to)}>{tab.label}</Button>)}
    </nav>}
    <Outlet />
  </>;
}
