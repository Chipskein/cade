# Pontos de Melhoria

## Ausência de integração contínua (CI)

* Não encontrei workflows do GitHub Actions na branch `dev`.
* Para um projeto com tantos testes e alvos de avaliação, um pipeline de CI que rode `make test`, `make eval-plan` e `make eval-retrieval` a cada push seria muito valioso para garantir que mudanças não quebrem funcionalidades.

## Complexidade de build e configuração inicial

* O build exige gcc, cmake, ninja, curl e, opcionalmente, o CUDA Toolkit.
* Além disso, é preciso baixar modelos (`make models`) e configurar manualmente o `config.json` com caminhos de repositórios, históricos e diretórios do Teams.
* Para um usuário não familiarizado com Go ou com o ecossistema llama.cpp, a barreira de entrada é alta.
* Um instalador mais amigável ou um script de configuração interativa ajudaria.

## Documentação em dois idiomas, mas inconsistente

* O README está em inglês, com uma versão em português.
* Porém, os rótulos da CLI estão majoritariamente em português, enquanto a documentação técnica está em inglês.
* Isso pode causar confusão para usuários que não falam português.
* Seria interessante padronizar o idioma da interface ou, pelo menos, documentar claramente essa escolha.

## Dependência de modelos externos e tamanho dos downloads

* O modelo de geração (Qwen2.5-3B) tem 2,1 GB, e o de embeddings, 344 MB.
* Embora sejam baixados localmente, isso pode ser um obstáculo em ambientes com pouco espaço ou conexão lenta.
* Não há menção a modelos menores ou alternativas mais leves no README.

## Falta de um exemplo de configuração completo

* O README mostra apenas um fragmento do `config.json`.
* Um arquivo de exemplo comentado (`config.example.json`) ou uma seção mais detalhada sobre cada campo ajudaria novos usuários.

## Possível acoplamento com o ambiente do desenvolvedor

* A integração com o Teams lê diretamente o IndexedDB do Chrome.
* Isso depende de caminhos específicos e pode quebrar com atualizações do navegador ou do Teams.
* O próprio README alerta que “se uma atualização do Teams renomear o que o cade lê, ingest teams falha”.
* Embora haja um comando de diagnóstico (`teams-schema`), a fragilidade é inerente.

## Falta de métricas de qualidade de código

* Não vi badges de cobertura de testes, análise estática (`golangci-lint`) ou status de build.
* Adicionar isso ao README daria mais confiança sobre a qualidade do projeto.
