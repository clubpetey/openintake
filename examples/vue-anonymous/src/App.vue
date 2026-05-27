<script setup lang="ts">
import { IntakeWidget } from '@intake/vue';
</script>

<template>
  <div class="demo-app">
    <header class="demo-app__header">
      <h1 class="demo-app__title">Intake Demo — Anonymous</h1>
      <p class="demo-app__subtitle">
        This is a minimal host app. Click the chat button in the bottom-right corner to open the support widget.
      </p>
    </header>

    <main class="demo-app__main">
      <p>Your app content goes here.</p>
    </main>

    <!-- The Intake widget: points at the local relay -->
    <!-- Security note: no API keys here — the relay handles all LLM communication -->
    <IntakeWidget
      relay-url="http://localhost:8080"
      :app-context="{ source: 'vue-anonymous-example', version: '0.1.0' }"
    />
  </div>
</template>

<style>
/* Global reset — keeps demo host app clean */
*, *::before, *::after { box-sizing: border-box; }

body {
  margin: 0;
  font-family: system-ui, sans-serif;
  background-color: #f8fafc;
  color: #1e293b;
}
</style>

<style scoped>
.demo-app {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}

.demo-app__header {
  padding: 32px 40px 24px;
  border-bottom: 1px solid #e2e8f0;
  background-color: #fff;
}

.demo-app__title {
  font-size: 24px;
  font-weight: 700;
  margin: 0 0 8px;
}

.demo-app__subtitle {
  font-size: 15px;
  color: #64748b;
  margin: 0;
}

.demo-app__main {
  flex: 1;
  padding: 40px;
}
</style>
